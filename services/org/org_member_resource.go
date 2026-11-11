package org

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	_ resource.Resource                = &OrgMemberResource{}
	_ resource.ResourceWithConfigure   = &OrgMemberResource{}
	_ resource.ResourceWithImportState = &OrgMemberResource{}
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
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	Email     types.String `tfsdk:"email"`
	Role      types.String `tfsdk:"role"`
	TeamIDs   types.Set    `tfsdk:"team_ids"`
	Status    types.String `tfsdk:"status"`
	OrgUserID types.String `tfsdk:"org_user_id"`
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
}

// set sends the plan's role and teams, and writes the answer back to plan.
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

	out, err := r.client.API.SetOrgMember(ctx, api.SetOrgMemberRequest{OrgID: orgID}, api.OrgMemberSetRequest{
		Email:   plan.Email.ValueString(),
		Role:    api.UserRole(plan.Role.ValueString()),
		TeamIDs: teamIDs,
	})
	if err != nil {
		tfutil.AddAPIError(&diags, failed, err)
		return diags
	}

	diags.Append(applyOrgMember(plan, &out.Member)...)
	return diags
}

// applyOrgMember writes what the server stores into m.
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
	return diags
}
