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

var teamPermLevels = []string{"none", "view", "edit", "admin"}

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
				},
			},
			"perm_level": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Project permission override applied to all team members: none, view, edit, or admin. Omitting this attribute lets the backend default apply (currently view) and the result is reflected in state.",
				Validators: []validator.String{
					stringvalidator.OneOf(teamPermLevels...),
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
