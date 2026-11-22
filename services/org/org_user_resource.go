package org

import (
	"context"
	"strconv"

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
	_ resource.Resource                = &OrgUserResource{}
	_ resource.ResourceWithConfigure   = &OrgUserResource{}
	_ resource.ResourceWithImportState = &OrgUserResource{}
	_ resource.ResourceWithModifyPlan  = &OrgUserResource{}
)

// OrgUserResource manages an organization membership row (`OrgUser`) that
// already exists: it reads it, changes its role, and removes it. It creates
// none (orgUserCreateRemoved). It stays for the memberships already in a
// state, and the next major version of the provider removes it.
type OrgUserResource struct {
	client *client.Client
}

type orgUserModel struct {
	ID     types.String `tfsdk:"id"`
	OrgID  types.String `tfsdk:"org_id"`
	UserID types.String `tfsdk:"user_id"`
	Role   types.String `tfsdk:"role"`
}

// orgUserDeprecation names the replacement of uptrace_org_user. The app
// removed the route that created a membership by user ID.
const orgUserDeprecation = "uptrace_org_user is deprecated, cannot create a membership, " +
	"and will be removed in the next major version of the provider. " +
	"Use uptrace_org_member, which takes the email address and invites a person who is not a member. " +
	"Move an existing membership with: terraform import uptrace_org_member.<name> <org_id>:<email>, " +
	"then remove uptrace_org_user from the state with terraform state rm."

func NewOrgUserResource() resource.Resource {
	return &OrgUserResource{}
}

func (r *OrgUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_user"
}

func (r *OrgUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an existing organization membership by its org user ID: reads it, changes its role, and removes it. " +
			"It cannot create a membership: the server no longer attaches a user to an organization by user ID, " +
			"so a new instance fails at plan time. Import a membership with `<org_id>:<org_user_id>`. " +
			"Deprecated: use `uptrace_org_member`, which takes the email address and invites a person who is not a member, " +
			"and the `uptrace_org_member` data source to read the `org_user_id` of a member.",
		DeprecationMessage: orgUserDeprecation,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "OrgUser ID. Use this value when another resource needs an `org_user_id` (e.g. `uptrace_team_user`).",
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
			"user_id": schema.StringAttribute{
				Required:    true,
				Description: "User ID to attach. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Required:    true,
				Description: "Organization role: owner, admin, member, viewer, billing_manager, or collaborator.",
				Validators:  []validator.String{userRoleValidator},
			},
		},
	}
}

func (r *OrgUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

// orgUserCreateRemoved is what a create answers. The app removed the operation
// that attached an account to an org by its user id: an administrator could
// attach any account of the deployment without its consent. A membership now
// begins with an invitation, which uptrace_org_member sends.
const orgUserCreateRemoved = "The server no longer attaches a user to an organization by user ID, " +
	"so uptrace_org_user cannot create a membership. " +
	"Use uptrace_org_member, which takes the email address and invites a person who is not a member. " +
	"A membership that this resource already manages can still be read, changed and removed."

// orgUserCreateRefused is the summary of the diagnostic that ModifyPlan and
// Create add.
const orgUserCreateRefused = "uptrace_org_user can no longer create a membership"

// ModifyPlan refuses at plan time every plan that needs a create: a new
// instance (no prior state) and a replacement, which a change of org_id or
// user_id asks for (RequiresReplace). A replacement would first remove the
// membership and then fail to create its successor, so it must fail before the
// apply removes anything. A value not known until the apply counts as a
// change, as it does for RequiresReplace. An update of the role and a destroy
// (a null plan) pass.
func (r *OrgUserResource) ModifyPlan(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) {
	if req.Plan.Raw.IsNull() {
		return
	}
	if req.State.Raw.IsNull() {
		resp.Diagnostics.AddError(orgUserCreateRefused, orgUserCreateRemoved)
		return
	}

	var plan, state orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.OrgID.Equal(state.OrgID) || !plan.UserID.Equal(state.UserID) {
		resp.Diagnostics.AddError(orgUserCreateRefused,
			"Changing org_id or user_id replaces the membership, and a replacement creates one. "+
				orgUserCreateRemoved)
	}
}

// Create refuses, as ModifyPlan does at plan time: the server operation is
// removed (orgUserCreateRemoved). Read, Update and Delete keep working for a
// membership already in the state.
func (r *OrgUserResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(orgUserCreateRefused, orgUserCreateRemoved)
}

func (r *OrgUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid id", err.Error())
		return
	}

	out, err := r.client.API.GetOrgUser(ctx, api.GetOrgUserRequest{OrgID: orgID, OrgUserID: orgUserID})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read org_user failed", err)
		return
	}

	state.UserID = types.StringValue(strconv.FormatUint(out.OrgUser.UserID, 10))
	state.Role = types.StringValue(string(out.OrgUser.Role))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid id", err.Error())
		return
	}

	tflog.Info(ctx, "updating org_user role", map[string]any{
		"org_user_id": state.ID.ValueString(),
		"role":        plan.Role.ValueString(),
	})

	out, err := r.client.API.UpdateOrgUserRole(ctx, api.UpdateOrgUserRoleRequest{OrgID: orgID, OrgUserID: orgUserID}, api.OrgUserRoleUpdateRequest{
		Role: api.UserRole(plan.Role.ValueString()),
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update org_user role failed", err)
		return
	}

	plan.ID = state.ID
	plan.Role = types.StringValue(string(out.User.Role))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}
	orgUserID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid id", err.Error())
		return
	}

	tflog.Info(ctx, "deleting org_user", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.RemoveOrgUser(ctx, api.RemoveOrgUserRequest{OrgID: orgID, OrgUserID: orgUserID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete org_user failed", err)
	}
}

// ImportState accepts "<org_id>:<org_user_id>". user_id is reconciled by Read.
func (r *OrgUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "id", Parse: tfutil.ParseUint64},
	)
}
