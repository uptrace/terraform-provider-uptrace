package org

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
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
	_ resource.Resource                = &OrgResource{}
	_ resource.ResourceWithConfigure   = &OrgResource{}
	_ resource.ResourceWithImportState = &OrgResource{}
)

// OrgResource manages an Uptrace organization.
type OrgResource struct {
	client *client.Client
}

type orgModel struct {
	ID     types.String  `tfsdk:"id"`
	Name   types.String  `tfsdk:"name"`
	Budget types.Float64 `tfsdk:"budget"`
}

func NewOrgResource() resource.Resource {
	return &OrgResource{}
}

func (r *OrgResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}

func (r *OrgResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Organization ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Organization name.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
				},
			},
			"budget": schema.Float64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Organization budget. Defaults to the system default when omitted during creation. Removing this attribute after setting it keeps the current API budget.",
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrgResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *OrgResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "creating org", map[string]any{"name": plan.Name.ValueString()})

	out, err := r.client.API.CreateOrg(ctx, &generated.CreateOrgRequestOptions{
		Body: &generated.OrgCreateRequest{Name: plan.Name.ValueString()},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create org failed", err)
		return
	}
	org := out.Org

	// The create request takes no budget: a configured one is a second call.
	if !plan.Budget.IsNull() && !plan.Budget.IsUnknown() {
		budgetOut, err := r.client.API.UpdateOrgBudget(ctx, &generated.UpdateOrgBudgetRequestOptions{
			PathParams: &generated.UpdateOrgBudgetPath{OrgID: org.ID},
			Body:       &generated.OrgUpdateBudgetRequest{Budget: plan.Budget.ValueFloat64()},
		})
		if err != nil {
			// The org exists: keep it in state so the next apply retries the budget.
			orgToModel(&org, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			tfutil.AddAPIError(&resp.Diagnostics, "set org budget failed", err)
			return
		}
		org = budgetOut.Org
	}

	orgToModel(&org, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org ID", err.Error())
		return
	}

	out, err := r.client.API.GetOrg(ctx, &generated.GetOrgRequestOptions{
		PathParams: &generated.GetOrgPath{OrgID: orgID},
	})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read org failed", err)
		return
	}

	orgToModel(&out.Org, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org ID", err.Error())
		return
	}

	tflog.Info(ctx, "updating org", map[string]any{"id": plan.ID.ValueString()})

	out, err := r.client.API.UpdateOrg(ctx, &generated.UpdateOrgRequestOptions{
		PathParams: &generated.UpdateOrgPath{OrgID: orgID},
		Body: &generated.OrgUpdateRequest{
			Name: plan.Name.ValueString(),
		},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update org failed", err)
		return
	}
	org := out.Org

	if !plan.Budget.IsNull() && !plan.Budget.IsUnknown() && !plan.Budget.Equal(state.Budget) {
		budgetOut, err := r.client.API.UpdateOrgBudget(ctx, &generated.UpdateOrgBudgetRequestOptions{
			PathParams: &generated.UpdateOrgBudgetPath{OrgID: orgID},
			Body: &generated.OrgUpdateBudgetRequest{
				Budget: plan.Budget.ValueFloat64(),
			},
		})
		if err != nil {
			tfutil.AddAPIError(&resp.Diagnostics, "update org budget failed", err)
			return
		}
		org = budgetOut.Org
	}

	orgToModel(&org, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org ID", err.Error())
		return
	}

	tflog.Info(ctx, "deleting org", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.DeleteOrg(ctx, &generated.DeleteOrgRequestOptions{
		PathParams: &generated.DeleteOrgPath{OrgID: orgID},
	})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete org failed", err)
	}
}

func (r *OrgResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func orgToModel(org *generated.Org, m *orgModel) {
	m.ID = types.StringValue(strconv.FormatUint(org.ID, 10))
	m.Name = types.StringValue(org.Name)
	m.Budget = types.Float64Value(org.Budget)
}
