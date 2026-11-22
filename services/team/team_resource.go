package team

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &TeamResource{}
	_ resource.ResourceWithConfigure   = &TeamResource{}
	_ resource.ResourceWithImportState = &TeamResource{}
)

// A team only adds project access, so it cannot hold none: the API refuses it.
var teamPermLevels = []string{"view", "edit", "admin"}

// teamPermLevelValidator accepts teamPermLevels. It refuses none with the
// steps that keep a configuration's denies. An app upgrade raised each team at
// none to view, unlinked its projects and moved its denies onto its members.
// Then the generic refusal leads an operator to set view, and the next apply
// links the projects again. The team then gives view on the projects it
// denied.
type teamPermLevelValidator struct{}

var teamPermLevelOneOf = stringvalidator.OneOf(teamPermLevels...)

func (teamPermLevelValidator) Description(ctx context.Context) string {
	return teamPermLevelOneOf.Description(ctx)
}

func (teamPermLevelValidator) MarkdownDescription(ctx context.Context) string {
	return teamPermLevelOneOf.MarkdownDescription(ctx)
}

func (teamPermLevelValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != "none" {
		teamPermLevelOneOf.ValidateString(ctx, req, resp)
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		`a team no longer holds "none"`,
		`A team only adds project access, so perm_level takes "view", "edit" or "admin". `+
			`The app upgrade raised each team at "none" to "view", unlinked its projects `+
			`and moved its denies onto its members as project overrides at "none". `+
			`To keep the denies: set perm_level to "view" or remove it, `+
			`remove the uptrace_team_project resources of this team, and set `+
			`project_overrides = { <project_id> = "none" } on the uptrace_org_member `+
			`of each person the team kept out. Linking the projects again gives the `+
			`team's members view on them.`,
	)
}

type TeamResource struct {
	client *client.Client
}

type teamModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	Name      types.String `tfsdk:"name"`
	PermLevel types.String `tfsdk:"perm_level"`
}

func NewTeamResource() resource.Resource {
	return &TeamResource{}
}

func (r *TeamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (r *TeamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace team within an organization. Teams are a Premium feature.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Team ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID this team belongs to. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Team name.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
					tfutil.TrimmedNameValidator{},
				},
			},
			"perm_level": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Permission level the team grants its members on each of its projects: view, edit, or admin. A team only adds access and never takes away what a member's role grants. Omitting this attribute lets the backend default apply (currently view) and the result is reflected in state.",
				Validators: []validator.String{
					teamPermLevelValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *TeamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *TeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	tflog.Info(ctx, "creating team", map[string]any{
		"org_id": plan.OrgID.ValueString(),
		"name":   plan.Name.ValueString(),
	})

	body := &api.TeamCreateRequest{
		Name: plan.Name.ValueString(),
	}
	if !plan.PermLevel.IsNull() && !plan.PermLevel.IsUnknown() {
		p := api.PermLevel(plan.PermLevel.ValueString())
		body.PermLevel = &p
	}

	out, err := r.client.API.CreateTeam(ctx, api.CreateTeamRequest{OrgID: orgID}, *body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create team failed", err)
		return
	}

	teamToModel(&out.Team, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	teamID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid team ID", err.Error())
		return
	}

	out, err := r.client.API.GetTeam(ctx, api.GetTeamRequest{OrgID: orgID, TeamID: teamID})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read team failed", err)
		return
	}

	teamToModel(&out.Team, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	teamID, err := strconv.ParseUint(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid team ID", err.Error())
		return
	}

	tflog.Info(ctx, "updating team", map[string]any{"id": plan.ID.ValueString()})

	name := plan.Name.ValueString()
	body := &api.TeamUpdateRequest{Name: &name}
	if !plan.PermLevel.IsNull() && !plan.PermLevel.IsUnknown() {
		p := api.PermLevel(plan.PermLevel.ValueString())
		body.PermLevel = &p
	}

	out, err := r.client.API.UpdateTeam(ctx, api.UpdateTeamRequest{OrgID: orgID, TeamID: teamID}, *body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update team failed", err)
		return
	}

	teamToModel(&out.Team, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	teamID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid team ID", err.Error())
		return
	}

	tflog.Info(ctx, "deleting team", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.DeleteTeam(ctx, api.DeleteTeamRequest{OrgID: orgID, TeamID: teamID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete team failed", err)
	}
}

// ImportState accepts "<org_id>:<team_id>".
func (r *TeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "team_id", Parse: tfutil.ParseUint64},
	)
}

func teamToModel(t *api.Team, m *teamModel) {
	m.ID = types.StringValue(strconv.FormatUint(t.ID, 10))
	m.OrgID = types.StringValue(strconv.FormatUint(t.OrgID, 10))
	m.Name = types.StringValue(t.Name)
	m.PermLevel = tfutil.EnumToValue(&t.PermLevel)
}
