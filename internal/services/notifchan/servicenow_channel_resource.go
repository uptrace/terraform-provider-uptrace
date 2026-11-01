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

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &ServicenowChannelResource{}
	_ resource.ResourceWithConfigure      = &ServicenowChannelResource{}
	_ resource.ResourceWithImportState    = &ServicenowChannelResource{}
	_ resource.ResourceWithValidateConfig = &ServicenowChannelResource{}
)

var (
	servicenowImpacts    = []string{"1", "2", "3"}
	servicenowUrgencies  = []string{"1", "2", "3"}
	servicenowSeverities = []string{"1", "2", "3", "4", "5"}
	servicenowNotifies   = []string{"1", "2"}
)

type ServicenowChannelResource struct {
	client *client.Client
}

func NewServicenowChannelResource() resource.Resource {
	return &ServicenowChannelResource{}
}

type servicenowChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	URL         types.String `tfsdk:"url"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Category    types.String `tfsdk:"category"`
	Subcategory types.String `tfsdk:"subcategory"`
	Impact      types.String `tfsdk:"impact"`
	Urgency     types.String `tfsdk:"urgency"`
	Severity    types.String `tfsdk:"severity"`
	CallerID    types.String `tfsdk:"caller_id"`
	Group       types.String `tfsdk:"group"`
	AssignedTo  types.String `tfsdk:"assigned_to"`
	OpenedBy    types.String `tfsdk:"opened_by"`
	Notify      types.String `tfsdk:"notify"`
	DueDate     types.String `tfsdk:"due_date"`
}

var servicenowCRUD = channelCRUD[servicenowChannelModel]{
	TypeName:  "servicenow_channel",
	ProjectID: func(m *servicenowChannelModel) types.String { return m.ProjectID },
	ID:        func(m *servicenowChannelModel) types.String { return m.ID },
	Name:      func(m *servicenowChannelModel) types.String { return m.Name },
	Build:     buildServicenowRequest,
	Apply:     applyServicenowToModel,
}

func (r *ServicenowChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servicenow_channel"
}

func (r *ServicenowChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["url"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "ServiceNow instance URL.",
	}
	attrs["username"] = schema.StringAttribute{
		Required:    true,
		Description: "ServiceNow username.",
	}
	attrs["password"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "ServiceNow password.",
	}
	attrs["category"] = schema.StringAttribute{Optional: true, Description: "Incident category."}
	attrs["subcategory"] = schema.StringAttribute{Optional: true, Description: "Incident subcategory."}
	attrs["impact"] = schema.StringAttribute{
		Optional:    true,
		Description: "Incident impact (1, 2, or 3).",
		Validators:  []validator.String{stringvalidator.OneOf(servicenowImpacts...)},
	}
	attrs["urgency"] = schema.StringAttribute{
		Optional:    true,
		Description: "Incident urgency (1, 2, or 3).",
		Validators:  []validator.String{stringvalidator.OneOf(servicenowUrgencies...)},
	}
	attrs["severity"] = schema.StringAttribute{
		Optional:    true,
		Description: "Incident severity (1-5).",
		Validators:  []validator.String{stringvalidator.OneOf(servicenowSeverities...)},
	}
	attrs["caller_id"] = schema.StringAttribute{Optional: true, Description: "Caller ID."}
	attrs["group"] = schema.StringAttribute{Optional: true, Description: "Assignment group."}
	attrs["assigned_to"] = schema.StringAttribute{Optional: true, Description: "Assigned to user."}
	attrs["opened_by"] = schema.StringAttribute{Optional: true, Description: "Opened by user."}
	attrs["notify"] = schema.StringAttribute{
		Optional:    true,
		Description: "Notify setting (1 or 2).",
		Validators:  []validator.String{stringvalidator.OneOf(servicenowNotifies...)},
	}
	attrs["due_date"] = schema.StringAttribute{Optional: true, Description: "Due date."}

	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace ServiceNow notification channel.",
		Attributes:  attrs,
	}
}

func (r *ServicenowChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *ServicenowChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg servicenowChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)
}

func (r *ServicenowChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan servicenowChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, servicenowCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServicenowChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state servicenowChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, servicenowCRUD)
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

func (r *ServicenowChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan servicenowChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, servicenowCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServicenowChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state servicenowChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, servicenowCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *ServicenowChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildServicenowRequest(ctx context.Context, m *servicenowChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "servicenow"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	p := generated.ServicenowParams{
		URL:      m.URL.ValueString(),
		Username: m.Username.ValueString(),
		Password: m.Password.ValueString(),
	}
	p.Category = valueStringToPtr(m.Category)
	p.Subcategory = valueStringToPtr(m.Subcategory)
	if !m.Impact.IsNull() && !m.Impact.IsUnknown() {
		v := generated.ServicenowImpact(m.Impact.ValueString())
		p.Impact = &v
	}
	if !m.Urgency.IsNull() && !m.Urgency.IsUnknown() {
		v := generated.ServicenowUrgency(m.Urgency.ValueString())
		p.Urgency = &v
	}
	if !m.Severity.IsNull() && !m.Severity.IsUnknown() {
		v := generated.ServicenowSeverity(m.Severity.ValueString())
		p.Severity = &v
	}
	p.CallerID = valueStringToPtr(m.CallerID)
	p.Group = valueStringToPtr(m.Group)
	p.AssignedTo = valueStringToPtr(m.AssignedTo)
	p.OpenedBy = valueStringToPtr(m.OpenedBy)
	if !m.Notify.IsNull() && !m.Notify.IsUnknown() {
		v := generated.ServicenowNotify(m.Notify.ValueString())
		p.Notify = &v
	}
	p.DueDate = valueStringToPtr(m.DueDate)

	body.Params = p

	return body, diags
}

func applyServicenowToModel(ctx context.Context, ch *channelResponse, dst *servicenowChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("servicenow", ch, &diags)
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

	var p generated.ServicenowParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode servicenow params", err.Error())
		return diags
	}
	dst.URL = tfutil.PreserveIfEmpty(p.URL, dst.URL)
	dst.Username = types.StringValue(p.Username)
	dst.Password = tfutil.PreserveIfEmpty(p.Password, dst.Password)
	dst.Category = tfutil.StringFromPtr(p.Category)
	dst.Subcategory = tfutil.StringFromPtr(p.Subcategory)
	dst.Impact = tfutil.EnumToValue(p.Impact)
	dst.Urgency = tfutil.EnumToValue(p.Urgency)
	dst.Severity = tfutil.EnumToValue(p.Severity)
	dst.CallerID = tfutil.StringFromPtr(p.CallerID)
	dst.Group = tfutil.StringFromPtr(p.Group)
	dst.AssignedTo = tfutil.StringFromPtr(p.AssignedTo)
	dst.OpenedBy = tfutil.StringFromPtr(p.OpenedBy)
	dst.Notify = tfutil.EnumToValue(p.Notify)
	dst.DueDate = tfutil.StringFromPtr(p.DueDate)

	return diags
}
