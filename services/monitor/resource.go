package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

// kind describes one monitor type. The server keeps a route per type
// (POST /monitors/{project_id}/<name>, PUT /monitors/{project_id}/{id}/<name>),
// a params shape per type, and states the type in every monitor it returns;
// everything else about a monitor is shared, and monitorResource holds it.
type kind struct {
	// name is the API type, the route segment and the resource suffix:
	// "threshold" serves uptrace_threshold_monitor.
	name        string
	description string

	params schema.SingleNestedAttribute

	// toAPI returns the params body for the configured params.
	toAPI func(ctx context.Context, params types.Object) (any, diag.Diagnostics)
	// fromAPI maps the params the server returned onto the attribute. prior
	// is the planned or stored value, null on import.
	fromAPI func(ctx context.Context, raw json.RawMessage, prior types.Object) (types.Object, diag.Diagnostics)

	// create and update decode body into the type's generated request and
	// send it on the type's route.
	create func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error)
	update func(ctx context.Context, c *client.Client, projectID uint32, monitorID uint64, body []byte) (*api.MonitorSummaryResponse, error)
}

func (k *kind) paramsAttrTypes() map[string]attr.Type {
	return k.params.GetType().(basetypes.ObjectType).AttrTypes
}

// monitorModel is the state of every monitor resource; Params holds the
// type's own block.
type monitorModel struct {
	ID               types.String `tfsdk:"id"`
	ProjectID        types.String `tfsdk:"project_id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	ChannelIDs       types.Set    `tfsdk:"channel_ids"`
	TrendAggFunc     types.String `tfsdk:"trend_agg_func"`
	TrendSensitivity types.String `tfsdk:"trend_sensitivity"`
	RepeatInterval   types.Object `tfsdk:"repeat_interval"`
	Status           types.String `tfsdk:"status"`
	Params           types.Object `tfsdk:"params"`
}

type repeatIntervalModel struct {
	Strategy types.String `tfsdk:"strategy"`
	Interval types.String `tfsdk:"interval"`
	Min      types.String `tfsdk:"min"`
	Max      types.String `tfsdk:"max"`
}

// repeatIntervalJSON is the wire form of every repeat strategy: default
// carries no duration, fixed an interval, linear and exponential a range.
type repeatIntervalJSON struct {
	Strategy string `json:"strategy"`
	Interval *int64 `json:"interval,omitempty"`
	Min      *int64 `json:"min,omitempty"`
	Max      *int64 `json:"max,omitempty"`
}

// monitorRequest is the body every typed create and update route takes. It
// lists only keys the spec states: the server refuses any other.
type monitorRequest struct {
	Name             string              `json:"name"`
	Description      *string             `json:"description,omitempty"`
	ChannelIDs       *[]uint64           `json:"channelIds,omitzero"`
	RepeatInterval   *repeatIntervalJSON `json:"repeatInterval,omitempty"`
	TrendAggFunc     *string             `json:"trendAggFunc,omitempty"`
	TrendSensitivity *string             `json:"trendSensitivity,omitempty"`
	Params           any                 `json:"params"`
}

// monitorResponse is a monitor as the server returns it, with the type's
// params left raw for the kind to decode.
type monitorResponse struct {
	ID               uint64             `json:"id"`
	Type             string             `json:"type"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Status           string             `json:"status"`
	ChannelIDs       []uint64           `json:"channelIds"`
	RepeatInterval   repeatIntervalJSON `json:"repeatInterval"`
	TrendAggFunc     string             `json:"trendAggFunc"`
	TrendSensitivity string             `json:"trendSensitivity"`
	Params           json.RawMessage    `json:"params"`
}

var repeatIntervalAttrTypes = map[string]attr.Type{
	"strategy": types.StringType,
	"interval": types.StringType,
	"min":      types.StringType,
	"max":      types.StringType,
}

type monitorResource struct {
	kind   *kind
	client *client.Client
}

var (
	_ resource.Resource                   = &monitorResource{}
	_ resource.ResourceWithConfigure      = &monitorResource{}
	_ resource.ResourceWithImportState    = &monitorResource{}
	_ resource.ResourceWithValidateConfig = &monitorResource{}
	_ resource.ResourceWithModifyPlan     = &monitorResource{}
)

func newResource(k *kind) func() resource.Resource {
	return func() resource.Resource { return &monitorResource{kind: k} }
}

func (r *monitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.name + "_monitor"
}

func (r *monitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: r.kind.description,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Monitor ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{
				Required:      true,
				Description:   "Project ID this monitor belongs to. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Monitor name. It may hold template variables, such as {{ .Attrs.host_name }}.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Alert description sent with each notification. It may hold template variables.",
			},
			"channel_ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Notification channel IDs, such as uptrace_telegram_channel.id. Omit to leave the links alone, as when channels name the monitor in their monitor_ids; set [] to detach every channel.",
			},
			"trend_agg_func": optionalComputedEnum(
				"How a series' points combine into the trend an alert shows.", api.TrendAggFuncSum,
				api.TrendAggFuncAvg, api.TrendAggFuncMedian, api.TrendAggFuncLast),
			"trend_sensitivity": optionalComputedEnum(
				"How much a value must move before the alert trend reports it.", api.TrendSensitivityVeryLow,
				api.TrendSensitivityLow, api.TrendSensitivityMedium,
				api.TrendSensitivityHigh, api.TrendSensitivityVeryHigh),
			"repeat_interval": schema.SingleNestedAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "How often a firing alert notifies again. Omit for the server's default strategy.",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"strategy": schema.StringAttribute{
						Required:    true,
						Description: "default, fixed (every interval), linear or exponential (from min up to max).",
						Validators: []validator.String{stringvalidator.OneOf(
							"default", "fixed", "linear", "exponential")},
					},
					"interval": durationAttribute(false, "Repeat interval of the fixed strategy."),
					"min":      durationAttribute(false, "First interval of the linear and exponential strategies."),
					"max":      durationAttribute(false, "Last interval of the linear and exponential strategies."),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Runtime status: active, paused, firing, no_data, warming_up or disabled.",
			},
			"params": r.kind.params,
		},
	}
}

func (r *monitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *monitorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.validateRepeatInterval(ctx, req, resp)
	r.validateWindowRule(ctx, req, resp)
}

// validateWindowRule refuses a window_rule and min_breach_points pair that the
// server folds on save (monitorspec normalizeWindowRule): an at_least count at
// or above num_eval_points becomes at_all_times with no count, and a count
// under any other rule is zeroed. A plan that held such a pair would end the
// apply inconsistent (DESIGN.md G3). An at_least rule without a count is
// refused too, as the server refuses it. It compares the count with the width
// only when the configuration sets num_eval_points; planWindowRule compares it
// with the planned width otherwise.
//
// The server reads a positive count without a rule as at_least (monitorspec
// RuleForBreachPoints), so that pair takes the at_least checks, and ModifyPlan
// plans the rule for it.
func (r *monitorResource) validateWindowRule(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if _, ok := r.kind.params.Attributes["window_rule"]; !ok {
		return
	}
	var params types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("params"), &params)...)
	if resp.Diagnostics.HasError() || params.IsNull() || params.IsUnknown() {
		return
	}
	var (
		rule, count, width = types.StringNull(), types.Int64Null(), types.Int64Null()
		p                  = path.Root("params")
	)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("window_rule"), &rule)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("min_breach_points"), &count)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("num_eval_points"), &width)...)
	if resp.Diagnostics.HasError() || rule.IsUnknown() || count.IsUnknown() || width.IsUnknown() {
		return
	}

	countImpliesRule := rule.IsNull() && !count.IsNull() && count.ValueInt64() > 0
	if rule.ValueString() != string(api.MetricWindowRuleAtLeast) && !countImpliesRule {
		if !count.IsNull() && count.ValueInt64() != 0 {
			resp.Diagnostics.AddAttributeError(p.AtName("min_breach_points"), "invalid min_breach_points",
				"only the at_least window rule counts breaching points; the server stores 0 under any other rule")
		}
		return
	}
	if count.IsNull() || count.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(p.AtName("min_breach_points"), "invalid min_breach_points",
			"the at_least window rule needs min_breach_points of 1 or more")
		return
	}
	// A configuration that omits the width plans the stored one, which only
	// the plan holds, so planWindowRule checks that case.
	if !width.IsNull() {
		refuseEveryPoint(&resp.Diagnostics, count.ValueInt64(), width.ValueInt64(), false)
	}
}

// refuseEveryPoint refuses an at_least count at or above the window width,
// which the server stores as the at_all_times rule. omitted reports that the
// configuration does not set the width.
func refuseEveryPoint(diags *diag.Diagnostics, count, width int64, omitted bool) {
	if count < width {
		return
	}
	detail := fmt.Sprintf("min_breach_points (%d) at or above num_eval_points (%d) is every point; "+
		"the server stores it as the at_all_times window rule, so write that rule instead", count, width)
	if omitted {
		detail += ", or set num_eval_points above the count. The configuration omits num_eval_points, " +
			"so the monitor keeps its stored width, or the server default for a new monitor"
	}
	diags.AddAttributeError(path.Root("params").AtName("min_breach_points"), "invalid min_breach_points", detail)
}

// validateRepeatInterval refuses a repeat_interval whose fields do not fit
// its strategy.
func (r *monitorResource) validateRepeatInterval(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var ri types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("repeat_interval"), &ri)...)
	if resp.Diagnostics.HasError() || ri.IsNull() || ri.IsUnknown() {
		return
	}
	var m repeatIntervalModel
	resp.Diagnostics.Append(ri.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || m.Strategy.IsUnknown() {
		return
	}
	set := func(v types.String) bool { return !v.IsNull() }
	p := path.Root("repeat_interval")
	switch m.Strategy.ValueString() {
	case "default":
		if set(m.Interval) || set(m.Min) || set(m.Max) {
			resp.Diagnostics.AddAttributeError(p, "invalid repeat_interval", "the default strategy takes no interval, min or max")
		}
	case "fixed":
		if !set(m.Interval) || set(m.Min) || set(m.Max) {
			resp.Diagnostics.AddAttributeError(p, "invalid repeat_interval", "the fixed strategy takes interval and no min or max")
		}
	case "linear", "exponential":
		if set(m.Interval) || !set(m.Min) || !set(m.Max) {
			resp.Diagnostics.AddAttributeError(p, "invalid repeat_interval",
				"the "+m.Strategy.ValueString()+" strategy takes min and max and no interval")
		}
	}
}

// ModifyPlan keeps the stored status when nothing else changes. Terraform
// proposes null for the computed attributes nested in params, so the framework
// sees a change and marks every computed attribute unknown; the
// UseStateForUnknown modifiers restore the others, but status has none, since
// a real update can change it. Without this, every plan of an unchanged
// monitor would show an update that only refreshes status.
//
// It first plans the window rule that a count without a rule means
// (planWindowRule), so the comparison with the state reads that rule.
func (r *monitorResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	r.planWindowRule(ctx, req, resp)
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}
	var plan, state monitorModel
	resp.Diagnostics.Append(resp.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !plan.Status.IsUnknown() {
		return
	}
	plan.Status = state.Status
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	if !resp.Plan.Raw.Equal(req.State.Raw) {
		// A real change: the server decides the status again.
		plan.Status = types.StringUnknown()
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

// planWindowRule plans window_rule = at_least for a configuration that sets a
// positive min_breach_points and no window_rule, which is how the server reads
// that pair (monitorspec RuleForBreachPoints). Without it, UseStateForUnknown
// plans the rule of the state. When that rule is not at_least, the server
// zeroes the count, and the apply ends inconsistent (DESIGN.md G3).
//
// The other way round, it plans min_breach_points = 0 for a configuration that
// sets a rule other than at_least and no count: the server stores 0 under such
// a rule, and UseStateForUnknown would plan the count of the state.
//
// It refuses an at_least count at or above the planned width when the
// configuration omits num_eval_points. UseStateForUnknown plans the stored
// width there, which validateWindowRule cannot see; a new monitor takes the
// server default.
func (r *monitorResource) planWindowRule(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if _, ok := r.kind.params.Attributes["window_rule"]; !ok {
		return
	}
	var params types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("params"), &params)...)
	if resp.Diagnostics.HasError() || params.IsNull() || params.IsUnknown() {
		return
	}
	var (
		rule, count, width = types.StringNull(), types.Int64Null(), types.Int64Null()
		p                  = path.Root("params")
	)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("window_rule"), &rule)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("min_breach_points"), &count)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p.AtName("num_eval_points"), &width)...)
	if resp.Diagnostics.HasError() || rule.IsUnknown() || count.IsUnknown() {
		return
	}
	counts := !count.IsNull() && count.ValueInt64() > 0 &&
		(rule.IsNull() || rule.ValueString() == string(api.MetricWindowRuleAtLeast))
	if counts && width.IsNull() {
		var planned types.Int64
		resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, p.AtName("num_eval_points"), &planned)...)
		if resp.Diagnostics.HasError() {
			return
		}
		evalPoints := int64(defaultNumEvalPoints)
		if !planned.IsNull() && !planned.IsUnknown() {
			evalPoints = planned.ValueInt64()
		}
		if refuseEveryPoint(&resp.Diagnostics, count.ValueInt64(), evalPoints, true); resp.Diagnostics.HasError() {
			return
		}
	}
	switch {
	case rule.IsNull() && !count.IsNull() && count.ValueInt64() > 0:
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, p.AtName("window_rule"),
			types.StringValue(string(api.MetricWindowRuleAtLeast)))...)
	case !rule.IsNull() && rule.ValueString() != string(api.MetricWindowRuleAtLeast) && count.IsNull():
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, p.AtName("min_breach_points"), types.Int64Value(0))...)
	}
}

func (r *monitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, ok := parseProjectID(plan.ProjectID, &resp.Diagnostics)
	if !ok {
		return
	}
	body, diags := r.requestBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating "+r.kind.name+" monitor", map[string]any{"project_id": projectID, "name": plan.Name.ValueString()})
	out, err := r.kind.create(ctx, r.client, projectID, body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create "+r.kind.name+" monitor failed", err)
		return
	}
	// The plan's spelling is the truth of a write, so no stored canon is
	// read; the server's canon is stored for the reads that follow.
	server, diags := r.apply(ctx, out.Monitor, &plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(server.store(ctx, resp.Private)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, ok := parseProjectID(state.ProjectID, &resp.Diagnostics)
	if !ok {
		return
	}
	monitorID, ok := parseMonitorID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	out, err := r.client.API.GetMonitor(ctx, api.GetMonitorRequest{ProjectID: projectID, MonitorID: monitorID})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read "+r.kind.name+" monitor failed", err)
		return
	}
	var canon queryCanon
	if req.Private != nil {
		var diags diag.Diagnostics
		canon, diags = readQueryCanon(ctx, req.Private)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	server, diags := r.apply(ctx, out.Monitor, &state, canon)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(server.store(ctx, resp.Private)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, ok := parseProjectID(plan.ProjectID, &resp.Diagnostics)
	if !ok {
		return
	}
	monitorID, ok := parseMonitorID(plan.ID, &resp.Diagnostics)
	if !ok {
		return
	}
	body, diags := r.requestBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "updating "+r.kind.name+" monitor", map[string]any{"id": monitorID})
	out, err := r.kind.update(ctx, r.client, projectID, monitorID, body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update "+r.kind.name+" monitor failed", err)
		return
	}
	server, diags := r.apply(ctx, out.Monitor, &plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(server.store(ctx, resp.Private)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, ok := parseProjectID(state.ProjectID, &resp.Diagnostics)
	if !ok {
		return
	}
	monitorID, ok := parseMonitorID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	tflog.Info(ctx, "deleting "+r.kind.name+" monitor", map[string]any{"id": monitorID})
	_, err := r.client.API.DeleteMonitor(ctx, api.DeleteMonitorRequest{ProjectID: projectID, MonitorID: monitorID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete "+r.kind.name+" monitor failed", err)
	}
}

// ImportState accepts "<project_id>:<monitor_id>". It sets channel_ids to an
// empty set, so the first read fills in the links the monitor has.
func (r *monitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "monitor_id", Parse: tfutil.ParseUint64},
	)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("channel_ids"), types.SetValueMust(types.StringType, nil))...)
}

// requestBody encodes the plan as the JSON body of the type's routes.
func (r *monitorResource) requestBody(ctx context.Context, m *monitorModel) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Send the description even when unset: removing it from the config must
	// clear it, and an omitted optional key may keep the stored value.
	description := m.Description.ValueString()
	body := monitorRequest{
		Name:             m.Name.ValueString(),
		Description:      &description,
		TrendAggFunc:     stringPtr(m.TrendAggFunc),
		TrendSensitivity: stringPtr(m.TrendSensitivity),
	}

	// A monitor and a channel share their links, and either side may declare
	// them. An unset channel_ids omits the key, which keeps the stored links, so
	// a channel's monitor_ids can own them; a set one replaces them, [] included.
	if !m.ChannelIDs.IsNull() && !m.ChannelIDs.IsUnknown() {
		channelIDs := []uint64{}
		body.ChannelIDs = &channelIDs
		var ids []string
		diags.Append(m.ChannelIDs.ElementsAs(ctx, &ids, false)...)
		for _, s := range ids {
			id, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				diags.AddAttributeError(path.Root("channel_ids"), "invalid channel ID",
					fmt.Sprintf("expected a decimal integer, got %q", s))
				continue
			}
			channelIDs = append(channelIDs, id)
		}
	}

	if !m.RepeatInterval.IsNull() && !m.RepeatInterval.IsUnknown() {
		var ri repeatIntervalModel
		diags.Append(m.RepeatInterval.As(ctx, &ri, basetypes.ObjectAsOptions{})...)
		out := &repeatIntervalJSON{Strategy: ri.Strategy.ValueString()}
		for _, f := range []struct {
			name string
			src  types.String
			dst  **int64
		}{
			{"interval", ri.Interval, &out.Interval},
			{"min", ri.Min, &out.Min},
			{"max", ri.Max, &out.Max},
		} {
			v, err := durationMillis(f.src)
			if err != nil {
				diags.AddAttributeError(path.Root("repeat_interval").AtName(f.name), "invalid duration", err.Error())
			}
			*f.dst = v
		}
		body.RepeatInterval = out
	}

	params, d := r.kind.toAPI(ctx, m.Params)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}
	body.Params = params

	b, err := json.Marshal(body)
	if err != nil {
		diags.AddError("failed to encode monitor", err.Error())
	}
	return b, diags
}

// apply maps a monitor the server returned onto m. monitor is any of the
// generated monitor unions; it is decoded through its JSON form.
// apply writes the monitor the server returned onto m, and returns the
// server's query texts (queryCanon). canon is what the server returned for
// them at the last write, or nil on a write and on a state from before the
// canon was stored: a query attribute whose server text differs from canon
// takes the server's value, so the plan shows an out-of-band change, and one
// that still matches keeps the configured spelling.
func (r *monitorResource) apply(ctx context.Context, monitor any, m *monitorModel, canon queryCanon) (queryCanon, diag.Diagnostics) {
	var diags diag.Diagnostics

	b, err := json.Marshal(monitor)
	if err != nil {
		diags.AddError("failed to decode monitor", err.Error())
		return nil, diags
	}
	var mon monitorResponse
	if err := json.Unmarshal(b, &mon); err != nil {
		diags.AddError("failed to decode monitor", err.Error())
		return nil, diags
	}
	if mon.Type != r.kind.name {
		diags.AddError("monitor type mismatch", fmt.Sprintf(
			"monitor %d is a %s monitor, not a %s monitor; import it as uptrace_%s_monitor",
			mon.ID, mon.Type, r.kind.name, mon.Type))
		return nil, diags
	}
	server, err := serverQueries(mon.Params)
	if err != nil {
		diags.AddError("failed to decode monitor params", err.Error())
		return nil, diags
	}

	m.ID = types.StringValue(strconv.FormatUint(mon.ID, 10))
	m.Name = types.StringValue(mon.Name)
	if mon.Description != "" || !m.Description.IsNull() {
		m.Description = types.StringValue(mon.Description)
	}
	m.Status = types.StringValue(mon.Status)
	m.TrendAggFunc = types.StringValue(mon.TrendAggFunc)
	m.TrendSensitivity = types.StringValue(mon.TrendSensitivity)

	// An unset channel_ids does not manage the links (see requestBody), so it
	// stays null whatever links the channels declared.
	if !m.ChannelIDs.IsNull() {
		ids := make([]string, len(mon.ChannelIDs))
		for i, id := range mon.ChannelIDs {
			ids[i] = strconv.FormatUint(id, 10)
		}
		v, d := types.SetValueFrom(ctx, types.StringType, ids)
		diags.Append(d...)
		m.ChannelIDs = v
	} else {
		m.ChannelIDs = types.SetNull(types.StringType)
	}

	var prior repeatIntervalModel
	if !m.RepeatInterval.IsNull() && !m.RepeatInterval.IsUnknown() {
		diags.Append(m.RepeatInterval.As(ctx, &prior, basetypes.ObjectAsOptions{})...)
	}
	ri := repeatIntervalModel{
		Strategy: types.StringValue(mon.RepeatInterval.Strategy),
		Interval: optionalDuration(mon.RepeatInterval.Interval, prior.Interval),
		Min:      optionalDuration(mon.RepeatInterval.Min, prior.Min),
		Max:      optionalDuration(mon.RepeatInterval.Max, prior.Max),
	}
	riValue, d := types.ObjectValueFrom(ctx, repeatIntervalAttrTypes, ri)
	diags.Append(d...)
	m.RepeatInterval = riValue

	priorParams, d := dropDriftedQueries(ctx, m.Params, server, canon)
	diags.Append(d...)
	params, d := r.kind.fromAPI(ctx, mon.Params, priorParams)
	diags.Append(d...)
	m.Params = params

	return server, diags
}

// queryCanon holds the query texts of a monitor as the server stores them,
// by the name of the params attribute that configures each: query, or
// good_query and total_query. The server rewrites a saved query to its
// canonical MQL text, so the configured spelling is kept in state while the
// server still returns the text it returned at the last write; the
// provider parses no MQL (DESIGN.md D3), so it compares the texts.
type queryCanon map[string]string

// queryCanonKey is the private state key that holds a queryCanon.
const queryCanonKey = "query_canon"

// serverQueries reads the query texts out of the params the server returned.
// A kind without a query has none.
func serverQueries(raw json.RawMessage) (queryCanon, error) {
	if len(raw) == 0 {
		return queryCanon{}, nil
	}
	var p struct {
		Query *string `json:"query"`
		Sli   *struct {
			GoodQuery  *string `json:"goodQuery"`
			TotalQuery *string `json:"totalQuery"`
		} `json:"sli"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	canon := queryCanon{}
	if p.Query != nil {
		canon["query"] = *p.Query
	}
	if p.Sli != nil {
		if p.Sli.GoodQuery != nil {
			canon["good_query"] = *p.Sli.GoodQuery
		}
		if p.Sli.TotalQuery != nil {
			canon["total_query"] = *p.Sli.TotalQuery
		}
	}
	return canon, nil
}

// privateState is the private state of a resource request or response
// (resource.ReadRequest.Private and the others), whose type the framework
// keeps internal.
type privateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// store writes c into the private state.
func (c queryCanon) store(ctx context.Context, private privateState) diag.Diagnostics {
	b, err := json.Marshal(c)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("failed to encode the monitor queries", err.Error())
		return diags
	}
	return private.SetKey(ctx, queryCanonKey, b)
}

// readQueryCanon reads the queryCanon the last write stored, or nil when the
// state holds none.
func readQueryCanon(ctx context.Context, private privateState) (queryCanon, diag.Diagnostics) {
	b, diags := private.GetKey(ctx, queryCanonKey)
	if diags.HasError() || len(b) == 0 {
		return nil, diags
	}
	var canon queryCanon
	if err := json.Unmarshal(b, &canon); err != nil {
		diags.AddError("failed to decode the stored monitor queries", err.Error())
		return nil, diags
	}
	return canon, diags
}

// dropDriftedQueries returns prior with each query attribute nulled whose
// server text differs from the text canon holds for it, so fromAPI takes the
// server's value. A nil canon, a null prior and an attribute the kind has not
// leave prior as it is.
func dropDriftedQueries(ctx context.Context, prior types.Object, server, canon queryCanon) (types.Object, diag.Diagnostics) {
	if canon == nil || prior.IsNull() || prior.IsUnknown() {
		return prior, nil
	}
	attrs := prior.Attributes()
	changed := false
	for name, text := range server {
		if _, ok := attrs[name]; !ok || canon[name] == text {
			continue
		}
		attrs[name] = types.StringNull()
		changed = true
	}
	if !changed {
		return prior, nil
	}
	return types.ObjectValue(prior.AttributeTypes(ctx), attrs)
}

func optionalDuration(ms *int64, prior types.String) types.String {
	if ms == nil {
		return types.StringNull()
	}
	return durationValue(*ms, prior)
}

func parseProjectID(v types.String, diags *diag.Diagnostics) (uint32, bool) {
	id, err := strconv.ParseUint(v.ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return 0, false
	}
	return uint32(id), true
}

func parseMonitorID(v types.String, diags *diag.Diagnostics) (uint64, bool) {
	id, err := strconv.ParseUint(v.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid monitor id", err.Error())
		return 0, false
	}
	return id, true
}
