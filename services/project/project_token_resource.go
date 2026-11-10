package project

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &ProjectTokenResource{}
	_ resource.ResourceWithConfigure   = &ProjectTokenResource{}
	_ resource.ResourceWithImportState = &ProjectTokenResource{}
)

type ProjectTokenResource struct {
	client *client.Client
}

type projectTokenModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Token     types.String `tfsdk:"token"`
	DSN       types.String `tfsdk:"dsn"`
}

func NewProjectTokenResource() resource.Resource {
	return &ProjectTokenResource{}
}

func (r *ProjectTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_token"
}

func (r *ProjectTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Token ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Project ID this token belongs to. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable token name.",
			},
			"token": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The secret token string. Generated server-side.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"dsn": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Computed DSN (ingest URL with token embedded).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ProjectTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *ProjectTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(plan.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tflog.Info(ctx, "creating project token", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
	})

	body := &generated.ProjectTokenCreateRequest{
		Name: plan.Name.ValueString(),
	}

	out, err := r.client.API.CreateProjectToken(ctx, &generated.CreateProjectTokenRequestOptions{
		PathParams: &generated.CreateProjectTokenPath{ProjectID: projectID},
		Body:       body,
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create project token failed", err)
		return
	}

	projectTokenToModel(&out.Token, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tokenID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid token ID", err.Error())
		return
	}

	out, err := r.client.API.GetProjectToken(ctx, &generated.GetProjectTokenRequestOptions{
		PathParams: &generated.GetProjectTokenPath{
			ProjectID: projectID,
			TokenID:   tokenID,
		},
	})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read project token failed", err)
		return
	}

	projectTokenToModel(&out.Token, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(plan.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tokenID, err := strconv.ParseUint(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid token ID", err.Error())
		return
	}

	tflog.Info(ctx, "updating project token", map[string]any{"id": plan.ID.ValueString()})

	name := plan.Name.ValueString()
	out, err := r.client.API.UpdateProjectToken(ctx, &generated.UpdateProjectTokenRequestOptions{
		PathParams: &generated.UpdateProjectTokenPath{
			ProjectID: projectID,
			TokenID:   tokenID,
		},
		Body: &generated.ProjectTokenUpdateRequest{
			Name: name,
		},
	})
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update project token failed", err)
		return
	}

	projectTokenToModel(&out.Token, &plan)

	plan.DSN = state.DSN
	plan.Token = state.Token
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ProjectID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project_id", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tokenID, err := strconv.ParseUint(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid token ID", err.Error())
		return
	}

	tflog.Info(ctx, "deleting project token", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.DeleteProjectToken(ctx, &generated.DeleteProjectTokenRequestOptions{
		PathParams: &generated.DeleteProjectTokenPath{
			ProjectID: projectID,
			TokenID:   tokenID,
		},
	})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete project token failed", err)
	}
}

// ImportState accepts "<project_id>:<token_id>".
func (r *ProjectTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "token_id", Parse: tfutil.ParseUint64},
	)
}

func projectTokenToModel(t *generated.ProjectToken, m *projectTokenModel) {
	m.ID = types.StringValue(strconv.FormatUint(t.ID, 10))
	m.Token = types.StringValue(t.Token)

	m.Name = types.StringValue(t.Name)
	m.DSN = types.StringValue(t.Dsn)
}
