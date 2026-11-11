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
	_ resource.Resource                = &TeamUserResource{}
	_ resource.ResourceWithConfigure   = &TeamUserResource{}
	_ resource.ResourceWithImportState = &TeamUserResource{}
)

type TeamUserResource struct {
	client *client.Client
}

type teamUserModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	TeamID    types.String `tfsdk:"team_id"`
	OrgUserID types.String `tfsdk:"org_user_id"`
}

func NewTeamUserResource() resource.Resource {
	return &TeamUserResource{}
}

func (r *TeamUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user"
}

func (r *TeamUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Adds an organization user to a team. Idempotent — creating twice is a no-op server-side. The org_user_id is the ID of the OrgUser record linking the user to the organization (not the User ID). " +
			"Do not use it for a person that `uptrace_org_member` manages: its `team_ids` is the exact set of that person's teams.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite identifier (equals org_user_id). Unique within the parent team.",
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
			"org_user_id": schema.StringAttribute{
				Required:    true,
				Description: "OrgUser ID to add to the team. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *TeamUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *TeamUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, orgUserID, diags := parseTeamUserIDs(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "adding user to team", map[string]any{
		"team_id":     plan.TeamID.ValueString(),
		"org_user_id": plan.OrgUserID.ValueString(),
	})

	_, err := r.client.API.AddTeamUser(ctx, api.AddTeamUserRequest{OrgID: orgID, TeamID: teamID, OrgUserID: orgUserID})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "add team user failed", err)
		return
	}

	plan.ID = plan.OrgUserID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, orgUserID, diags := parseTeamUserIDs(state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.API.ListTeamUsers(ctx, api.ListTeamUsersRequest{OrgID: orgID, TeamID: teamID})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "list team users failed", err)
		return
	}

	for _, u := range out.Users {
		if u.OrgUserID == orgUserID {
			state.ID = state.OrgUserID
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *TeamUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes are RequiresReplace; Update is never reached in practice.
	var plan teamUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, teamID, orgUserID, diags := parseTeamUserIDs(state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "removing user from team", map[string]any{
		"team_id":     state.TeamID.ValueString(),
		"org_user_id": state.OrgUserID.ValueString(),
	})

	_, err := r.client.API.RemoveTeamUser(ctx, api.RemoveTeamUserRequest{OrgID: orgID, TeamID: teamID, OrgUserID: orgUserID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "remove team user failed", err)
	}
}

// ImportState accepts "<org_id>:<team_id>:<org_user_id>". id mirrors
// org_user_id (see Create), so the junction helper writes both.
func (r *TeamUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateJunctionID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "team_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "org_user_id", Parse: tfutil.ParseUint64},
	)
}

func parseTeamUserIDs(m teamUserModel) (orgID uint64, teamID uint64, orgUserID uint64, diags diag.Diagnostics) {
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
	orgUserID, err = strconv.ParseUint(m.OrgUserID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid org_user_id", err.Error())
		return
	}
	return
}
