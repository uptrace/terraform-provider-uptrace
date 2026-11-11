package org

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
)

// OrgUserResource manages an organization membership row (`OrgUser`).
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
// deprecated the route it calls, and removes it after one app minor release.
const orgUserDeprecation = "uptrace_org_user is deprecated and will be removed in the next major version of the provider. " +
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
		Description: "Adds an existing user to an organization with a role. Idempotent — creating twice updates the role server-side. Pair with `uptrace_user` to manage the underlying user. " +
			"Deprecated: use `uptrace_org_member`, which takes the email address instead of a user ID.",
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

func (r *OrgUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, userID, diags := parseOrgUserCreateIDs(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating org_user", map[string]any{
		"org_id":  plan.OrgID.ValueString(),
		"user_id": plan.UserID.ValueString(),
		"role":    plan.Role.ValueString(),
	})

	//nolint:staticcheck // SA1019: the resource is deprecated with the operation; uptrace_org_member replaces both, and the next major provider version removes them.
	out, err := r.client.API.CreateOrgUser(ctx, api.CreateOrgUserRequest{OrgID: orgID}, api.OrgUserCreateRequest{
		UserID: userID,
		Role:   api.UserRole(plan.Role.ValueString()),
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create org_user failed", err)
		return
	}

	plan.ID = types.StringValue(strconv.FormatUint(out.OrgUser.ID, 10))
	plan.Role = types.StringValue(string(out.OrgUser.Role))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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

func parseOrgUserCreateIDs(m orgUserModel) (orgID uint64, userID uint64, diags diag.Diagnostics) {
	var err error
	orgID, err = strconv.ParseUint(m.OrgID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid org_id", err.Error())
		return
	}
	userID, err = strconv.ParseUint(m.UserID.ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid user_id", err.Error())
		return
	}
	return
}
