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
	_ resource.Resource                = &TeamsChannelResource{}
	_ resource.ResourceWithConfigure   = &TeamsChannelResource{}
	_ resource.ResourceWithImportState = &TeamsChannelResource{}
)

type TeamsChannelResource struct {
	client *client.Client
}

func NewTeamsChannelResource() resource.Resource {
	return &TeamsChannelResource{}
}

type teamsChannelModel struct {
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

var teamsCRUD = channelCRUD[teamsChannelModel]{
	TypeName:  "teams_channel",
	ProjectID: func(m *teamsChannelModel) types.String { return m.ProjectID },
	ID:        func(m *teamsChannelModel) types.String { return m.ID },
	Name:      func(m *teamsChannelModel) types.String { return m.Name },
	Build:     buildTeamsRequest,
	Apply:     applyTeamsToModel,
}

func (r *TeamsChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_teams_channel"
}

func (r *TeamsChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["webhook_url"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Microsoft Teams webhook URL.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Microsoft Teams notification channel.",
		Attributes:  attrs,
	}
}

func (r *TeamsChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *TeamsChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamsChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, teamsCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamsChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamsChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, teamsCRUD)
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

func (r *TeamsChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan teamsChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, teamsCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamsChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamsChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, teamsCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *TeamsChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildTeamsRequest(ctx context.Context, m *teamsChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "teams"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	body.Params = generated.TeamsParams{
		WebhookURL: m.WebhookURL.ValueString(),
	}

	return body, diags
}

func applyTeamsToModel(ctx context.Context, ch *channelResponse, dst *teamsChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("teams", ch, &diags)
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
	var p generated.TeamsParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode teams params", err.Error())
		return diags
	}
	dst.WebhookURL = tfutil.PreserveIfEmpty(p.WebhookURL, dst.WebhookURL)

	return diags
}
