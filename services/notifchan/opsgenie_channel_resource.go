package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &OpsgenieChannelResource{}
	_ resource.ResourceWithConfigure   = &OpsgenieChannelResource{}
	_ resource.ResourceWithImportState = &OpsgenieChannelResource{}
)

var opsgeniePriorities = []string{"P1", "P2", "P3", "P4", "P5"}

type OpsgenieChannelResource struct {
	client *client.Client
}

func NewOpsgenieChannelResource() resource.Resource {
	return &OpsgenieChannelResource{}
}

type opsgenieChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	APIKey   types.String `tfsdk:"api_key"`
	Priority types.String `tfsdk:"priority"`
	Provider types.String `tfsdk:"api_provider"`
	Endpoint types.String `tfsdk:"endpoint"`
}

var opsgenieCRUD = channelCRUD[opsgenieChannelModel]{
	TypeName:  "opsgenie_channel",
	ProjectID: func(m *opsgenieChannelModel) types.String { return m.ProjectID },
	ID:        func(m *opsgenieChannelModel) types.String { return m.ID },
	Name:      func(m *opsgenieChannelModel) types.String { return m.Name },
	Build:     buildOpsgenieRequest,
	Apply:     applyOpsgenieToModel,
}

func (r *OpsgenieChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_opsgenie_channel"
}

func (r *OpsgenieChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["api_key"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Opsgenie API key.",
	}
	attrs["priority"] = schema.StringAttribute{
		Required:    true,
		Description: "Opsgenie alert priority (P1–P5).",
		Validators: []validator.String{
			stringvalidator.OneOf(opsgeniePriorities...),
		},
	}
	attrs["api_provider"] = schema.StringAttribute{
		Optional:    true,
		Computed:    true,
		Default:     stringdefault.StaticString(string(generated.OpsgenieProviderOpsgenie)),
		Description: "Alert API that receives the alerts: opsgenie (the default), jsm (Jira Service Management), or custom.",
		Validators: []validator.String{
			stringvalidator.OneOf(
				string(generated.OpsgenieProviderOpsgenie),
				string(generated.OpsgenieProviderJSM),
				string(generated.OpsgenieProviderCustom),
			),
		},
	}
	attrs["endpoint"] = schema.StringAttribute{
		Optional:    true,
		Description: "Alert API URL. Required when api_provider is custom; leave unset for opsgenie and jsm.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Opsgenie notification channel.",
		Attributes:  attrs,
	}
}

func (r *OpsgenieChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *OpsgenieChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan opsgenieChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, opsgenieCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OpsgenieChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state opsgenieChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, opsgenieCRUD)
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

func (r *OpsgenieChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan opsgenieChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, opsgenieCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OpsgenieChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state opsgenieChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, opsgenieCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *OpsgenieChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildOpsgenieRequest(ctx context.Context, m *opsgenieChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "opsgenie"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	body.Params = generated.OpsgenieParams{
		APIKey:   m.APIKey.ValueString(),
		Priority: generated.OpsgeniePriority(m.Priority.ValueString()),
		Provider: generated.OpsgenieProvider(m.Provider.ValueString()),
		Endpoint: m.Endpoint.ValueString(),
	}

	return body, diags
}

func applyOpsgenieToModel(ctx context.Context, ch *channelResponse, dst *opsgenieChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("opsgenie", ch, &diags)
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

	var p generated.OpsgenieParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode opsgenie params", err.Error())
		return diags
	}
	dst.APIKey = tfutil.PreserveIfEmpty(p.APIKey, dst.APIKey)
	dst.Priority = types.StringValue(string(p.Priority))
	if p.Provider != "" {
		dst.Provider = types.StringValue(string(p.Provider))
	}
	if p.Endpoint != "" {
		dst.Endpoint = types.StringValue(p.Endpoint)
	} else {
		dst.Endpoint = types.StringNull()
	}

	return diags
}
