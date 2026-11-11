package team

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &TeamProjectResource{}
	_ resource.ResourceWithConfigure   = &TeamProjectResource{}
	_ resource.ResourceWithImportState = &TeamProjectResource{}
)

type TeamProjectResource struct {
	client *client.Client
}

type teamProjectModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	TeamID    types.String `tfsdk:"team_id"`
	ProjectID types.String `tfsdk:"project_id"`
}

func NewTeamProjectResource() resource.Resource {
	return &TeamProjectResource{}
}

func (r *TeamProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_project"
}

func (r *TeamProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants a team access to a project. Idempotent — creating twice is a no-op server-side. Delete revokes access without touching the team or project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite identifier (equals project_id). Unique within the parent team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID the team belongs to. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_id": schema.StringAttribute{
				Required:    true,
				Description: "Team ID. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Project ID to grant the team access to. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *TeamProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *TeamProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamProjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, projectID, diags := parseTeamProjectIDs(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "adding project to team", map[string]any{
		"team_id":    plan.TeamID.ValueString(),
		"project_id": plan.ProjectID.ValueString(),
	})

	_, err := r.client.API.AddTeamProject(ctx, api.AddTeamProjectRequest{OrgID: orgID, TeamID: teamID, ProjectID: projectID})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "add team project failed", err)
		return
	}

	plan.ID = plan.ProjectID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamProjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, projectID, diags := parseTeamProjectIDs(state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.API.ListTeamProjects(ctx, api.ListTeamProjectsRequest{OrgID: orgID, TeamID: teamID})
	if err != nil {
		// Parent team (or org) is gone — the membership is implicitly gone too.
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "list team projects failed", err)
		return
	}

	for _, p := range out.Projects {
		if p.ID == projectID {
			state.ID = state.ProjectID
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Project no longer associated with the team.
	resp.State.RemoveResource(ctx)
}

func (r *TeamProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes are RequiresReplace, so Update is never reached in
	// practice. Pass the plan through to state to satisfy the interface.
	var plan teamProjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamProjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, projectID, diags := parseTeamProjectIDs(state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "removing project from team", map[string]any{
		"team_id":    state.TeamID.ValueString(),
		"project_id": state.ProjectID.ValueString(),
	})

	_, err := r.client.API.RemoveTeamProject(ctx, api.RemoveTeamProjectRequest{OrgID: orgID, TeamID: teamID, ProjectID: projectID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "remove team project failed", err)
	}
}

// ImportState accepts "<org_id>:<team_id>:<project_id>". id mirrors
// project_id (see Create), so the junction helper writes both.
func (r *TeamProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateJunctionID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "team_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
	)
}

func parseTeamProjectIDs(m teamProjectModel) (orgID uint64, teamID uint64, projectID uint32, diags diag.Diagnostics) {
	var err error
	orgID, err = strconv.ParseUint(m.OrgID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid org_id", err.Error())
		return
	}
	teamID, err = strconv.ParseUint(m.TeamID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid team_id", err.Error())
		return
	}
	pid64, err := strconv.ParseUint(m.ProjectID.ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return
	}
	projectID = uint32(pid64)
	return
}
