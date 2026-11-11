package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &PagerdutyChannelResource{}
	_ resource.ResourceWithConfigure   = &PagerdutyChannelResource{}
	_ resource.ResourceWithImportState = &PagerdutyChannelResource{}
)

var pagerdutySeverities = []string{"critical", "error", "warning", "info"}

type PagerdutyChannelResource struct {
	client *client.Client
}

func NewPagerdutyChannelResource() resource.Resource {
	return &PagerdutyChannelResource{}
}

type pagerdutyChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	RoutingKey types.String `tfsdk:"routing_key"`
	Severity   types.String `tfsdk:"severity"`
}

var pagerdutyCRUD = channelCRUD[pagerdutyChannelModel]{
	TypeName:  "pagerduty_channel",
	ProjectID: func(m *pagerdutyChannelModel) types.String { return m.ProjectID },
	ID:        func(m *pagerdutyChannelModel) types.String { return m.ID },
	Name:      func(m *pagerdutyChannelModel) types.String { return m.Name },
	Build:     buildPagerdutyRequest,
	Apply:     applyPagerdutyToModel,
}

func (r *PagerdutyChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pagerduty_channel"
}

func (r *PagerdutyChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["routing_key"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "PagerDuty routing key.",
	}
	attrs["severity"] = schema.StringAttribute{
		Required:    true,
		Description: "PagerDuty severity: critical, error, warning, or info.",
		Validators: []validator.String{
			stringvalidator.OneOf(pagerdutySeverities...),
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace PagerDuty notification channel.",
		Attributes:  attrs,
	}
}

func (r *PagerdutyChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *PagerdutyChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pagerdutyChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, pagerdutyCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PagerdutyChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pagerdutyChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, pagerdutyCRUD)
	resp.Diagnostics.Append(diags...)
	if remove {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PagerdutyChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pagerdutyChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, pagerdutyCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PagerdutyChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pagerdutyChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, pagerdutyCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *PagerdutyChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildPagerdutyRequest(ctx context.Context, m *pagerdutyChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "pagerduty"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	body.Params = api.PagerdutyParams{
		RoutingKey: m.RoutingKey.ValueString(),
		Severity:   api.PagerdutySeverity(m.Severity.ValueString()),
	}

	return body, diags
}

func applyPagerdutyToModel(ctx context.Context, ch *channelResponse, dst *pagerdutyChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("pagerduty", ch, &diags)
	if diags.HasError() {
		return diags
	}

	diags.Append(applyShared(ctx, ch, sharedFields{
		ID: &dst.ID, Name: &dst.Name, Status: &dst.Status,
		MatchAll: &dst.MatchAll, Condition: &dst.Condition,
		Priorities: &dst.Priorities, MonitorIDs: &dst.MonitorIDs,
	})...)
	if diags.HasError() {
		return diags
	}

	var p api.PagerdutyParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode pagerduty params", err.Error())
		return diags
	}
	dst.RoutingKey = tfutil.PreserveIfEmpty(p.RoutingKey, dst.RoutingKey)
	dst.Severity = types.StringValue(string(p.Severity))

	return diags
}
