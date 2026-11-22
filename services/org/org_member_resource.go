package org

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                   = &OrgMemberResource{}
	_ resource.ResourceWithConfigure      = &OrgMemberResource{}
	_ resource.ResourceWithImportState    = &OrgMemberResource{}
	_ resource.ResourceWithValidateConfig = &OrgMemberResource{}
)

// userRoleValidator accepts the organization roles. uptrace_org_member and
// uptrace_org_user share it, so the two list the same roles.
var userRoleValidator = stringvalidator.OneOf(
	string(api.UserRoleOwner),
	string(api.UserRoleAdmin),
	string(api.UserRoleMember),
	string(api.UserRoleViewer),
	string(api.UserRoleBillingManager),
	string(api.UserRoleCollaborator),
)

// OrgMemberResource manages one person's membership of an organization by the
// email address. The server keeps it pending while the invitation waits and
// active after the person accepts it, and answers the same for every address
// that is not a member, so the provider never learns whether an account
// exists.
type OrgMemberResource struct {
	client *client.Client
}

type orgMemberModel struct {
	ID      types.String `tfsdk:"id"`
	OrgID   types.String `tfsdk:"org_id"`
	Email   types.String `tfsdk:"email"`
	Role    types.String `tfsdk:"role"`
	TeamIDs types.Set    `tfsdk:"team_ids"`
	// ProjectOverrides is null while the configuration leaves the overrides
	// to the server, and the exact set otherwise. It implements ADR-0466: a
	// null map sends no key (projectOverridesFromMap) and Read leaves it null
	// (applyOrgMember), so adding the attribute changes no existing
	// configuration.
	ProjectOverrides types.Map    `tfsdk:"project_overrides"`
	Status           types.String `tfsdk:"status"`
	OrgUserID        types.String `tfsdk:"org_user_id"`
}

// NewOrgMemberResource is the constructor the org service registers.
func NewOrgMemberResource() resource.Resource {
	return &OrgMemberResource{}
}

func (r *OrgMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_member"
}

func (r *OrgMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Makes the person at an email address a member of an organization with a role and a set of teams. " +
			"A person who is not a member gets an invitation email, and the membership stays `pending` until they accept it; " +
			"it is then `active`. An existing and a missing account get the same invitation. " +
			"A change of the role or the teams of a pending membership sends no new email. " +
			"`team_ids` is the exact set of teams of the person: do not also manage the same person with `uptrace_team_user`. " +
			"`project_overrides`, when set, is the exact set of the person's project overrides; " +
			"a pending membership holds them until the invitation is accepted. " +
			"Destroying the resource removes the member, or cancels the invitation. " +
			"Requires an app version that serves `/internal/v1/orgs/{org_id}/members/by-email`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "`<org_id>:<email>`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "The person's email address, lowercase and trimmed. Changing this forces recreation.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
					tfutil.LowercaseEmailValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Required:    true,
				Description: "Organization role: owner, admin, member, viewer, billing_manager, or collaborator.",
				Validators:  []validator.String{userRoleValidator},
			},
			"team_ids": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Description: "The exact set of team IDs of the person in the organization. " +
					"Omitted or empty, the person is in no team. While the membership is pending, the teams the person joins on acceptance.",
			},
			"project_overrides": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The exact project overrides of the person, keyed by project ID: `none` keeps the person out of the project, " +
					"and `view`, `edit` or `admin` raises the level the role grants on it. " +
					"A level below the one the role grants on every project is refused: an `owner` or an `admin` takes only `admin` or `none`. " +
					"An empty map removes every override. Omitted, the overrides on the server are left as they are and not read. " +
					"While the membership is pending, the overrides the person gets on acceptance. " +
					"Requires an app version that serves `projectOverrides` on `/internal/v1/orgs/{org_id}/members/by-email`.",
				Validators: []validator.Map{
					mapvalidator.ValueStringsAre(permLevelValidator),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "`pending` while the invitation waits, `active` after the person accepts it.",
			},
			"org_user_id": schema.StringAttribute{
				Computed:    true,
				Description: "The org user ID of an active membership. Null while the membership is pending.",
			},
		},
	}
}

// ValidateConfig refuses at plan time a project override that the server
// refuses at apply: a project ID that is not a uint32, and a level below the
// one the role grants on every project, other than none. It also refuses a
// project ID with a leading zero, which the state writes back without it
// (terraform:QS-1). It judges only known values, so a role or a map that another
// resource computes is left to the server.
func (r *OrgMemberResource) ValidateConfig(
	ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse,
) {
	var config orgMemberModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.ProjectOverrides.IsNull() || config.ProjectOverrides.IsUnknown() {
		return
	}

	for key, value := range config.ProjectOverrides.Elements() {
		at := path.Root("project_overrides").AtMapKey(key)
		projectID, err := strconv.ParseUint(key, 10, 32)
		if err != nil {
			resp.Diagnostics.AddAttributeError(at, "invalid project ID",
				fmt.Sprintf("a key of project_overrides is a project ID, got %q", key))
			continue
		}
		// The state writes the key in decimal with no leading zero, so any
		// other spelling would differ from the plan after the apply.
		if canonical := strconv.FormatUint(projectID, 10); canonical != key {
			resp.Diagnostics.AddAttributeError(at, "invalid project ID",
				fmt.Sprintf("write project ID %s as %q, with no leading zero, got %q", canonical, canonical, key))
			continue
		}
		level, ok := value.(types.String)
		if !ok || level.IsNull() || level.IsUnknown() || config.Role.IsNull() || config.Role.IsUnknown() {
			continue
		}
		role, override := api.UserRole(config.Role.ValueString()), api.PermLevel(level.ValueString())
		if !isValidOverride(role, override) {
			resp.Diagnostics.AddAttributeError(at, "project override below the role",
				overrideRefusal(role, override))
		}
	}
}

func (r *OrgMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *OrgMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.set(ctx, &plan, "create org_member failed")...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	out, err := r.client.API.GetOrgMember(ctx, api.GetOrgMemberRequest{OrgID: orgID, Email: state.Email.ValueString()})
	if err != nil {
		// No member and no live invitation: an expired or canceled one, or a
		// person removed outside Terraform. The next apply invites again.
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read org_member failed", err)
		return
	}

	resp.Diagnostics.Append(applyOrgMember(&state, &out.Member)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan orgMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.set(ctx, &plan, "update org_member failed")...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	tflog.Info(ctx, "deleting org_member", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.RemoveOrgMember(ctx, api.RemoveOrgMemberRequest{OrgID: orgID, Email: state.Email.ValueString()})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete org_member failed", err)
	}
}

// ImportState accepts "<org_id>:<email>". The email is the rest of the ID after
// the first ":", so any address parses, and the whole ID is the `id`. The
// compound helpers of tfutil split on every ":", so this resource parses its
// own ID.
func (r *OrgMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	orgID, email, ok := strings.Cut(req.ID, ":")
	if !ok || orgID == "" || email == "" {
		resp.Diagnostics.AddError("invalid import ID",
			fmt.Sprintf("expected format <org_id>:<email>, got %q", req.ID))
		return
	}
	if err := tfutil.ParseUint64(orgID); err != nil {
		resp.Diagnostics.AddError("invalid org_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_id"), orgID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), email)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	// An empty map, not null, so the Read that follows fills the overrides the
	// server holds. A configuration that omits them plans them away once, and
	// that apply leaves the server as it is (ADR-0466).
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_overrides"),
		types.MapValueMust(types.StringType, map[string]attr.Value{}))...)
}

// set sends the plan's role, teams and project overrides, and writes the answer
// back to plan. A null project_overrides sends no key, so the server keeps the
// overrides it holds.
func (r *OrgMemberResource) set(ctx context.Context, plan *orgMemberModel, failed string) diag.Diagnostics {
	var diags diag.Diagnostics

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		diags.AddAttributeError(path.Root("org_id"), "invalid org_id", err.Error())
		return diags
	}
	teamIDs, d := tfutil.Uint64sFromSet(ctx, path.Root("team_ids"), plan.TeamIDs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	slices.Sort(teamIDs)

	tflog.Info(ctx, "setting org_member", map[string]any{
		"org_id": plan.OrgID.ValueString(),
		"role":   plan.Role.ValueString(),
	})

	overrides, d := projectOverridesFromMap(plan.ProjectOverrides)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	out, err := r.client.API.SetOrgMember(ctx, api.SetOrgMemberRequest{OrgID: orgID}, api.OrgMemberSetRequest{
		Email:            plan.Email.ValueString(),
		Role:             api.UserRole(plan.Role.ValueString()),
		TeamIDs:          teamIDs,
		ProjectOverrides: overrides,
	})
	if err != nil {
		tfutil.AddAPIError(&diags, failed, err)
		return diags
	}

	diags.Append(applyOrgMember(plan, &out.Member)...)
	return diags
}

// applyOrgMember writes what the server stores into m. It leaves a null
// project_overrides null: the configuration does not manage them (ADR-0466).
func applyOrgMember(m *orgMemberModel, member *api.OrgMember) diag.Diagnostics {
	m.ID = types.StringValue(m.OrgID.ValueString() + ":" + member.Email)
	m.Email = types.StringValue(member.Email)
	m.Role = types.StringValue(string(member.Role))
	m.Status = types.StringValue(string(member.Status))
	m.OrgUserID = types.StringNull()
	if member.OrgUserID != nil {
		m.OrgUserID = types.StringValue(strconv.FormatUint(*member.OrgUserID, 10))
	}

	set, diags := tfutil.SetFromUint64s(member.TeamIDs)
	m.TeamIDs = set

	if !m.ProjectOverrides.IsNull() {
		levels := make(map[string]attr.Value, len(member.ProjectOverrides))
		for _, override := range member.ProjectOverrides {
			levels[strconv.FormatUint(uint64(override.ProjectID), 10)] = types.StringValue(string(override.PermLevel))
		}
		overrides, d := types.MapValue(types.StringType, levels)
		diags.Append(d...)
		m.ProjectOverrides = overrides
	}
	return diags
}

// projectOverridesFromMap reads project_overrides into the request's list,
// sorted by project ID. A null map is nil, which the client leaves out of the
// body.
func projectOverridesFromMap(m types.Map) (*[]api.OrgMemberProjectOverride, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m.IsNull() || m.IsUnknown() {
		return nil, diags
	}
	out := make([]api.OrgMemberProjectOverride, 0, len(m.Elements()))
	for key, value := range m.Elements() {
		projectID, err := strconv.ParseUint(key, 10, 32)
		if err != nil {
			diags.AddAttributeError(path.Root("project_overrides").AtMapKey(key), "invalid project ID", err.Error())
			continue
		}
		level, ok := value.(types.String)
		if !ok {
			diags.AddAttributeError(path.Root("project_overrides").AtMapKey(key), "invalid level",
				fmt.Sprintf("want a string, got %T", value))
			continue
		}
		out = append(out, api.OrgMemberProjectOverride{
			ProjectID: uint32(projectID),
			PermLevel: api.PermLevel(level.ValueString()),
		})
	}
	slices.SortFunc(out, func(a, b api.OrgMemberProjectOverride) int {
		return cmp.Compare(a.ProjectID, b.ProjectID)
	})
	return &out, diags
}

// permLevelValidator accepts the project override levels.
var permLevelValidator = stringvalidator.OneOf(
	string(api.PermLevelNone),
	string(api.PermLevelView),
	string(api.PermLevelEdit),
	string(api.PermLevelAdmin),
)

// overrideRefusal says why the server refuses an override at level under
// role: the server's reason (app core/tenancy checkOverrideLevel) without the
// project, which the attribute path names.
// testAccCheckOverrideLevelsMatchServer compares the two.
func overrideRefusal(role api.UserRole, level api.PermLevel) string {
	roleLevel := rolePermLevel(role)
	return fmt.Sprintf("must be %s or none, because the %s role grants %s on every project (got %s)",
		roleLevel, role, roleLevel, level)
}

// rolePermLevel is the level role grants on every project: the permLevel
// table of the contract's UserRole, which task generate:api writes into
// terraform/api as it writes it into the app (ADR-0525). A
// role the table omits grants none. TestAccOrgMember_projectOverrides checks
// the contract this provider was built from against the server it talks to.
func rolePermLevel(role api.UserRole) api.PermLevel {
	if level, ok := api.UserRolePermLevel(role); ok {
		return level
	}
	return api.PermLevelNone
}

// isValidOverride reports whether the server takes an override at level under
// role: none, or a level at or above the role's (ADR-0989).
func isValidOverride(role api.UserRole, level api.PermLevel) bool {
	return level == api.PermLevelNone || permRank(level) >= permRank(rolePermLevel(role))
}

// permRank is the rank of level in the contract's PermLevel table, which
// task generate:api writes into terraform/api. An unknown level ranks below
// none, so the level validator, not this check, reports it.
func permRank(level api.PermLevel) int {
	if rank, ok := api.PermLevelRank(level); ok {
		return rank
	}
	return -1
}
