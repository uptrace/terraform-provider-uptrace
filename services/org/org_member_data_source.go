package org

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ datasource.DataSource              = &OrgMemberDataSource{}
	_ datasource.DataSourceWithConfigure = &OrgMemberDataSource{}
)

// OrgMemberDataSource reads one person's membership of an organization by the
// email address, as the uptrace_org_member resource does, without managing
// it. It is how a configuration learns the org_user_id of a member it does not
// manage, such as the provider's own account, which owns every organization it
// creates. It reads the same operation as the resource (get_org_member).
type OrgMemberDataSource struct {
	client *client.Client
}

type orgMemberDataModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	Email     types.String `tfsdk:"email"`
	Role      types.String `tfsdk:"role"`
	TeamIDs   types.Set    `tfsdk:"team_ids"`
	Status    types.String `tfsdk:"status"`
	OrgUserID types.String `tfsdk:"org_user_id"`
}

// orgMemberNotFound is the detail of a read that finds no membership. The
// operation answers 404 for an address bound to an SSO provider too, so the
// message names that case: the person may well be a member.
const orgMemberNotFound = "The organization has no membership and no live invitation for this email address. " +
	"A membership that an SSO provider is bound to is not read by email either, " +
	"so a person who signs in through the organization's SSO provider is not found here."

// NewOrgMemberDataSource is the constructor the org service registers.
func NewOrgMemberDataSource() datasource.DataSource {
	return &OrgMemberDataSource{}
}

func (d *OrgMemberDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_member"
}

func (d *OrgMemberDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads one person's membership of an organization by the email address: `active` for a member, " +
			"`pending` while an invitation waits. Use it to get the `org_user_id` of a member that this configuration does not manage, " +
			"such as the provider's own account, which owns each organization it creates. " +
			"The read fails when the address has no membership and no live invitation, " +
			"and for a membership that an SSO provider is bound to, which is not read by email. Requires org admin. " +
			"Requires an app version that serves `/internal/v1/orgs/{org_id}/members/by-email`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "`<org_id>:<email>`.",
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID.",
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "The person's email address, lowercase and trimmed.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
					tfutil.LowercaseEmailValidator{},
				},
			},
			"role": schema.StringAttribute{
				Computed:    true,
				Description: "Organization role: owner, admin, member, viewer, billing_manager, or collaborator.",
			},
			"team_ids": schema.SetAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "The team IDs of the person in the organization. While the membership is pending, the teams the person joins on acceptance.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "`pending` while the invitation waits, `active` after the person accepts it.",
			},
			"org_user_id": schema.StringAttribute{
				Computed: true,
				Description: "The org user ID of an active membership, for an `org_user_id` such as that of `uptrace_team_user`. " +
					"Null while the membership is pending.",
			},
		},
	}
}

func (d *OrgMemberDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (d *OrgMemberDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data orgMemberDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(data.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	out, err := d.client.API.GetOrgMember(ctx, api.GetOrgMemberRequest{OrgID: orgID, Email: data.Email.ValueString()})
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("org_member not found", orgMemberNotFound)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read org_member failed", err)
		return
	}

	resp.Diagnostics.Append(applyOrgMemberData(&data, &out.Member)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyOrgMemberData copies a membership into the data source's model. It
// shares the fields of applyOrgMember, which the resource uses, and has no
// project overrides: the resource reads them only when its configuration
// manages them.
func applyOrgMemberData(m *orgMemberDataModel, member *api.OrgMember) diag.Diagnostics {
	full := orgMemberModel{OrgID: m.OrgID, ProjectOverrides: types.MapNull(types.StringType)}
	diags := applyOrgMember(&full, member)
	m.ID = full.ID
	m.Email = full.Email
	m.Role = full.Role
	m.TeamIDs = full.TeamIDs
	m.Status = full.Status
	m.OrgUserID = full.OrgUserID
	return diags
}
