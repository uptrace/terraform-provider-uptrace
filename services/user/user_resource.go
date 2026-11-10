package user

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

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource              = &UserResource{}
	_ resource.ResourceWithConfigure = &UserResource{}
)

// UserResource manages an Uptrace user (orgless). Creation issues an
// invite via POST /internal/v1/invites, which pre-creates the User row
// and returns its numeric ID.
type UserResource struct {
	client *client.Client
}

type userModel struct {
	ID    types.String `tfsdk:"id"`
	Email types.String `tfsdk:"email"`
}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a global Uptrace user. Creation issues an orgless invite that pre-creates the User row server-side and returns its ID. Pair with `uptrace_org_user` to grant org membership.\n\nThe caller must be authenticated and have access to at least one organization with an active subscription; otherwise the backend returns 402. The backend sends an account-invitation email to the recipient with a link to confirm the email and set a password. If the email already maps to a confirmed user, no invite is created and the existing user ID is returned — `apply` adopts existing users by email, so importing is unnecessary and not supported.\n\nEmail is immutable; changing it forces recreation. Delete removes the resource from Terraform state only — the underlying User row and any pending invite remain on the server, since the API has no global user-delete endpoint. Drift detection is not implemented: out-of-band changes to the underlying user are not reflected in plan output.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Numeric user ID returned by the backend.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "Email address of the user. Must be lowercase and trimmed.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
					tfutil.LowercaseEmailValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating user (orgless invite)", map[string]any{"email": plan.Email.ValueString()})

	out, err := r.client.API.CreateOrglessInvite(ctx, &generated.CreateOrglessInviteRequestOptions{
		Body: &generated.OrglessUserInviteCreateRequest{
			Email: plan.Email.ValueString(),
		},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create user invite failed", err)
		return
	}

	plan.ID = types.StringValue(strconv.FormatUint(out.User.ID, 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All mutable attributes are RequiresReplace; Update is unreachable in practice.
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Info(ctx, "deleting user (no-op; backend has no global user-delete endpoint)",
		map[string]any{"id": state.ID.ValueString()})
}
