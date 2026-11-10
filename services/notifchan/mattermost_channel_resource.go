package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &MattermostChannelResource{}
	_ resource.ResourceWithConfigure   = &MattermostChannelResource{}
	_ resource.ResourceWithImportState = &MattermostChannelResource{}
)

type MattermostChannelResource struct {
	client *client.Client
}

func NewMattermostChannelResource() resource.Resource {
	return &MattermostChannelResource{}
}

type mattermostChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	WebhookURL types.String `tfsdk:"webhook_url"`
}

var mattermostCRUD = channelCRUD[mattermostChannelModel]{
	TypeName:  "mattermost_channel",
	ProjectID: func(m *mattermostChannelModel) types.String { return m.ProjectID },
	ID:        func(m *mattermostChannelModel) types.String { return m.ID },
	Name:      func(m *mattermostChannelModel) types.String { return m.Name },
	Build:     buildMattermostRequest,
	Apply:     applyMattermostToModel,
}

func (r *MattermostChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mattermost_channel"
}

func (r *MattermostChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["webhook_url"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Mattermost webhook URL.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Mattermost notification channel.",
		Attributes:  attrs,
	}
}

func (r *MattermostChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *MattermostChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mattermostChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, mattermostCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MattermostChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mattermostChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, mattermostCRUD)
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

func (r *MattermostChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan mattermostChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, mattermostCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MattermostChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mattermostChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, mattermostCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *MattermostChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildMattermostRequest(ctx context.Context, m *mattermostChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "mattermost"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	body.Params = generated.MattermostParams{
		WebhookURL: m.WebhookURL.ValueString(),
	}

	return body, diags
}

func applyMattermostToModel(ctx context.Context, ch *channelResponse, dst *mattermostChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("mattermost", ch, &diags)
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

	if len(ch.Params) == 0 || string(ch.Params) == "null" {
		dst.WebhookURL = tfutil.PreferPrior(dst.WebhookURL, types.StringNull())
		return diags
	}
	var p generated.MattermostParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode mattermost params", err.Error())
		return diags
	}
	dst.WebhookURL = tfutil.PreserveIfEmpty(p.WebhookURL, dst.WebhookURL)

	return diags
}
