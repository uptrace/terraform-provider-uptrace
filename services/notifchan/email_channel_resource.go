package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                   = &EmailChannelResource{}
	_ resource.ResourceWithConfigure      = &EmailChannelResource{}
	_ resource.ResourceWithImportState    = &EmailChannelResource{}
	_ resource.ResourceWithValidateConfig = &EmailChannelResource{}
)

type EmailChannelResource struct {
	client *client.Client
}

func NewEmailChannelResource() resource.Resource {
	return &EmailChannelResource{}
}

type emailChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	Everyone types.Bool `tfsdk:"everyone"`
	TeamIDs  types.Set  `tfsdk:"team_ids"`
	UserIDs  types.Set  `tfsdk:"user_ids"`
}

var emailCRUD = channelCRUD[emailChannelModel]{
	TypeName:  "email_channel",
	ProjectID: func(m *emailChannelModel) types.String { return m.ProjectID },
	ID:        func(m *emailChannelModel) types.String { return m.ID },
	Name:      func(m *emailChannelModel) types.String { return m.Name },
	Build:     buildEmailRequest,
	Apply:     applyEmailToModel,
}

func (r *EmailChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_channel"
}

func (r *EmailChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptyIDs := types.SetValueMust(types.StringType, []attr.Value{})

	attrs := sharedAttributes()
	attrs["everyone"] = schema.BoolAttribute{
		Optional:    true,
		Computed:    true,
		Default:     booldefault.StaticBool(false),
		Description: "Email every member of the project, resolved on each delivery, so a later member is included. Conflicts with team_ids and user_ids.",
	}
	// team_ids and user_ids default to an empty set rather than null: the server
	// returns both lists as [] when everyone is set, and a null default would
	// then differ from the state on every read.
	attrs["team_ids"] = schema.SetAttribute{
		Optional:    true,
		Computed:    true,
		ElementType: types.StringType,
		Default:     setdefault.StaticValue(emptyIDs),
		Description: "IDs of the teams whose members get the email. Set everyone, or at least one of team_ids and user_ids.",
	}
	attrs["user_ids"] = schema.SetAttribute{
		Optional:    true,
		Computed:    true,
		ElementType: types.StringType,
		Default:     setdefault.StaticValue(emptyIDs),
		Description: "IDs of the users who get the email. Set everyone, or at least one of team_ids and user_ids.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace email notification channel. It addresses project members, not addresses: everyone, or the listed teams and users. Only members of the channel's project receive the email.",
		Attributes:  attrs,
	}
}

func (r *EmailChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *EmailChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg emailChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateEmailAudience(cfg.Everyone, cfg.TeamIDs, cfg.UserIDs, &resp.Diagnostics)
}

// validateEmailAudience holds the server's audience rule at plan time. The
// server clears both lists when everyone is set, so a config with both would
// show a diff on every plan; with neither, the server refuses the channel.
// Unknown values pass: the rule is checked again on apply.
func validateEmailAudience(everyone types.Bool, teamIDs, userIDs types.Set, diags *diag.Diagnostics) {
	if everyone.IsUnknown() || teamIDs.IsUnknown() || userIDs.IsUnknown() {
		return
	}
	hasLists := len(teamIDs.Elements()) > 0 || len(userIDs.Elements()) > 0
	switch {
	case everyone.ValueBool() && hasLists:
		diags.AddAttributeError(
			path.Root("everyone"),
			"everyone conflicts with team_ids and user_ids",
			"everyone addresses every project member, and the server drops team_ids and user_ids beside it. Set everyone, or list teams and users.",
		)
	case !everyone.ValueBool() && !hasLists:
		diags.AddAttributeError(
			path.Root("everyone"),
			"the channel addresses no one",
			"Set everyone = true, or at least one of team_ids and user_ids.",
		)
	}
}

func (r *EmailChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, emailCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EmailChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emailChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, emailCRUD)
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

func (r *EmailChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan emailChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, emailCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EmailChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emailChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, emailCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *EmailChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildEmailRequest(ctx context.Context, m *emailChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "email"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	// The spec requires both lists, so an empty one goes as [], never null.
	teamIDs, d := tfutil.Uint64sFromSet(ctx, path.Root("team_ids"), m.TeamIDs)
	diags.Append(d...)
	userIDs, d := tfutil.Uint64sFromSet(ctx, path.Root("user_ids"), m.UserIDs)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}
	body.Params = api.EmailParams{
		Everyone: m.Everyone.ValueBool(),
		TeamIDs:  teamIDs,
		UserIDs:  userIDs,
	}

	return body, diags
}

func applyEmailToModel(ctx context.Context, ch *channelResponse, dst *emailChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("email", ch, &diags)
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

	var p api.EmailParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode email params", err.Error())
		return diags
	}
	dst.Everyone = types.BoolValue(p.Everyone)
	teamIDs, d := tfutil.SetFromUint64s(p.TeamIDs)
	diags.Append(d...)
	userIDs, d := tfutil.SetFromUint64s(p.UserIDs)
	diags.Append(d...)
	dst.TeamIDs = teamIDs
	dst.UserIDs = userIDs

	return diags
}
