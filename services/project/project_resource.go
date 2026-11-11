package project

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	str2duration "github.com/xhit/go-str2duration/v2"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithConfigure   = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

// ProjectResource manages an Uptrace project scoped under an organization.
type ProjectResource struct {
	client *client.Client
}

type projectModel struct {
	ID    types.String `tfsdk:"id"`
	OrgID types.String `tfsdk:"org_id"`
	Name  types.String `tfsdk:"name"`

	GroupByEnv          types.Bool `tfsdk:"group_by_env"`
	GroupFuncsByService types.Bool `tfsdk:"group_funcs_by_service"`

	SemconvVersion types.String `tfsdk:"semconv_version"`

	SkipMonitorStatusEmails   types.Bool `tfsdk:"skip_monitor_status_emails"`
	SkipMonitorDisabledEmails types.Bool `tfsdk:"skip_monitor_disabled_emails"`

	SpanTimeRange  types.String `tfsdk:"span_time_range"`
	LogTimeRange   types.String `tfsdk:"log_time_range"`
	EventTimeRange types.String `tfsdk:"event_time_range"`

	SpanRetention    types.String `tfsdk:"span_retention"`
	LogRetention     types.String `tfsdk:"log_retention"`
	EventRetention   types.String `tfsdk:"event_retention"`
	MetricRetention  types.String `tfsdk:"metric_retention"`
	ProfileRetention types.String `tfsdk:"profile_retention"`
}

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

func (r *ProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optionalComputedBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Description: desc,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		}
	}
	semconvVersionAttr := schema.StringAttribute{
		Optional:    true,
		Computed:    true,
		Description: "Semantic conventions version. One of: none, v1.25.0, v1.33.0.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
		Validators: []validator.String{
			stringvalidator.OneOf("none", "v1.25.0", "v1.33.0"),
		},
	}
	optionalDuration := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:    true,
			Description: desc + ` Duration string; stdlib units (ns, us, ms, s, m, h) plus d (day) and w (week).`,
			Validators: []validator.String{
				durationStringValidator{},
			},
		}
	}
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Project ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Required:    true,
				Description: "Organization ID that owns this project. Changing this forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Project name.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
				},
			},
			"group_by_env":                 optionalComputedBool("Group spans by deployment environment."),
			"group_funcs_by_service":       optionalComputedBool("Group functions by service name."),
			"semconv_version":              semconvVersionAttr,
			"skip_monitor_status_emails":   optionalComputedBool("Send no email when a monitor of the project changes status."),
			"skip_monitor_disabled_emails": optionalComputedBool("Send no email when the server disables a monitor of the project."),

			"span_time_range":  optionalDuration("Default span query time range. Omit to use the server default."),
			"log_time_range":   optionalDuration("Default log query time range. Omit to use the server default."),
			"event_time_range": optionalDuration("Default event query time range. Omit to use the server default."),

			"span_retention":    optionalDuration("Span retention duration. Omit to use the server default."),
			"log_retention":     optionalDuration("Log retention duration. Omit to use the server default."),
			"event_retention":   optionalDuration("Event retention duration. Omit to use the server default."),
			"metric_retention":  optionalDuration("Metric retention duration. Omit to use the server default."),
			"profile_retention": optionalDuration("Profile retention duration. Omit to use the server default."),
		},
	}
}

func (r *ProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := strconv.ParseUint(plan.OrgID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("invalid org_id", err.Error())
		return
	}

	tflog.Info(ctx, "creating project", map[string]any{
		"org_id": plan.OrgID.ValueString(),
		"name":   plan.Name.ValueString(),
	})

	fields, err := projectRequestFields(&plan)
	if err != nil {
		resp.Diagnostics.AddError("invalid project configuration", err.Error())
		return
	}
	body := api.ProjectCreateRequest(fields)
	out, err := r.client.API.CreateProject(ctx, api.CreateProjectRequest{OrgID: orgID}, body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "create project failed", err)
		return
	}
	project := out.Project

	if !r.patchViewSettings(ctx, project.ID, &plan, &project, &resp.Diagnostics) {
		// The project exists: keep it in state so the next apply retries the patch.
		projectToModel(&project, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	projectToModel(&project, &plan)
	// Preserve the org_id supplied in config in case the API omits it in the response.
	plan.OrgID = types.StringValue(strconv.FormatUint(orgID, 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project ID", err.Error())
		return
	}
	projectID := uint32(projectID64)

	out, err := r.client.API.GetProject(ctx, api.GetProjectRequest{ProjectID: projectID})
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tfutil.AddAPIError(&resp.Diagnostics, "read project failed", err)
		return
	}

	prevOrgID := state.OrgID
	projectToModel(&out.Project, &state)
	if state.OrgID.IsNull() {
		state.OrgID = prevOrgID
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(plan.ID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project ID", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tflog.Info(ctx, "updating project", map[string]any{"id": plan.ID.ValueString()})

	fields, err := projectRequestFields(&plan)
	if err != nil {
		resp.Diagnostics.AddError("invalid project configuration", err.Error())
		return
	}
	body := api.ProjectUpdateRequest{
		Name:                      fields.Name,
		GroupByEnv:                fields.GroupByEnv,
		GroupFuncsByService:       fields.GroupFuncsByService,
		SkipMonitorStatusEmails:   fields.SkipMonitorStatusEmails,
		SkipMonitorDisabledEmails: fields.SkipMonitorDisabledEmails,
		SemconvVersion:            fields.SemconvVersion,
		SpanRetention:             fields.SpanRetention,
		LogRetention:              fields.LogRetention,
		EventRetention:            fields.EventRetention,
		MetricRetention:           fields.MetricRetention,
		ProfileRetention:          fields.ProfileRetention,
	}
	out, err := r.client.API.UpdateProject(ctx, api.UpdateProjectRequest{ProjectID: projectID}, body)
	if err != nil {
		tfutil.AddAPIError(&resp.Diagnostics, "update project failed", err)
		return
	}
	project := out.Project
	if !r.patchViewSettings(ctx, projectID, &plan, &project, &resp.Diagnostics) {
		return
	}

	prevOrgID := plan.OrgID
	projectToModel(&project, &plan)
	if plan.OrgID.IsNull() {
		plan.OrgID = prevOrgID
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID64, err := strconv.ParseUint(state.ID.ValueString(), 10, 32)
	if err != nil {
		resp.Diagnostics.AddError("invalid project ID", err.Error())
		return
	}
	projectID := uint32(projectID64)

	tflog.Info(ctx, "deleting project", map[string]any{"id": state.ID.ValueString()})

	_, err = r.client.API.DeleteProject(ctx, api.DeleteProjectRequest{ProjectID: projectID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&resp.Diagnostics, "delete project failed", err)
	}
}

// ImportState accepts "<orgID>:<projectID>" so org_id is populated even when
// the API response omits orgId.
func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "org_id", Parse: tfutil.ParseUint64},
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
	)
}

// projectRequestFields builds the fields a project create and a project update
// share. The server refuses a key its request does not list, so every field
// here exists in both.
func projectRequestFields(m *projectModel) (api.ProjectCreateRequest, error) {
	body := api.ProjectCreateRequest{
		Name: m.Name.ValueString(),
	}

	body.GroupByEnv = boolPtrIfSet(m.GroupByEnv)
	body.GroupFuncsByService = boolPtrIfSet(m.GroupFuncsByService)
	body.SkipMonitorStatusEmails = boolPtrIfSet(m.SkipMonitorStatusEmails)
	body.SkipMonitorDisabledEmails = boolPtrIfSet(m.SkipMonitorDisabledEmails)

	if !m.SemconvVersion.IsNull() && !m.SemconvVersion.IsUnknown() {
		v := api.SemconvVersion(m.SemconvVersion.ValueString())
		body.SemconvVersion = &v
	}

	for _, f := range []struct {
		name string
		src  types.String
		dst  **int64
	}{
		{"span_retention", m.SpanRetention, &body.SpanRetention},
		{"log_retention", m.LogRetention, &body.LogRetention},
		{"event_retention", m.EventRetention, &body.EventRetention},
		{"metric_retention", m.MetricRetention, &body.MetricRetention},
		{"profile_retention", m.ProfileRetention, &body.ProfileRetention},
	} {
		v, err := durationMillisPtrIfSet(f.src)
		if err != nil {
			return body, fmt.Errorf("%s: %w", f.name, err)
		}
		*f.dst = v
	}

	return body, nil
}

// viewSettingsRequest returns the PATCH body for the default query time
// ranges, or nil when the config sets none of them. They are view settings,
// which the server keeps apart from the project's create and update requests.
func viewSettingsRequest(m *projectModel) (*api.ProjectViewSettingsRequest, error) {
	body := &api.ProjectViewSettingsRequest{}
	set := false
	for _, f := range []struct {
		name string
		src  types.String
		dst  **int64
	}{
		{"span_time_range", m.SpanTimeRange, &body.SpanTimeRange},
		{"log_time_range", m.LogTimeRange, &body.LogTimeRange},
		{"event_time_range", m.EventTimeRange, &body.EventTimeRange},
	} {
		v, err := durationMillisPtrIfSet(f.src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		if v != nil {
			*f.dst = v
			set = true
		}
	}
	if !set {
		return nil, nil
	}
	return body, nil
}

// patchViewSettings applies the configured time ranges to the project and
// refreshes project from the answer. It reports false after adding an error.
func (r *ProjectResource) patchViewSettings(
	ctx context.Context, projectID uint32, m *projectModel, project *api.Project, diags *diag.Diagnostics,
) bool {
	body, err := viewSettingsRequest(m)
	if err != nil {
		diags.AddError("invalid project configuration", err.Error())
		return false
	}
	if body == nil {
		return true
	}
	out, err := r.client.API.PatchProject(ctx, api.PatchProjectRequest{ProjectID: projectID}, *body)
	if err != nil {
		tfutil.AddAPIError(diags, "update project time ranges failed", err)
		return false
	}
	*project = out.Project
	return true
}

func boolPtrIfSet(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func durationMillisPtrIfSet(v types.String) (*int64, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	d, err := str2duration.ParseDuration(v.ValueString())
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", v.ValueString(), err)
	}
	ms := d.Milliseconds()
	return &ms, nil
}

type durationStringValidator struct{}

func (durationStringValidator) Description(_ context.Context) string {
	return `must be a duration string (e.g. "24h", "30d", "1w3d")`
}

func (v durationStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (durationStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	d, err := str2duration.ParseDuration(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid duration",
			fmt.Sprintf("%q is not a valid duration string: %s", req.ConfigValue.ValueString(), err))
		return
	}
	if d < 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid duration",
			fmt.Sprintf("%q must not be negative", req.ConfigValue.ValueString()))
	}
}

func projectToModel(p *api.Project, m *projectModel) {
	m.ID = types.StringValue(strconv.FormatInt(int64(p.ID), 10))
	m.Name = types.StringValue(p.Name)
	m.OrgID = types.StringValue(strconv.FormatUint(p.OrgID, 10))

	m.GroupByEnv = types.BoolValue(p.GroupByEnv)
	m.GroupFuncsByService = types.BoolValue(p.GroupFuncsByService)
	m.SkipMonitorStatusEmails = types.BoolValue(p.SkipMonitorStatusEmails)
	m.SkipMonitorDisabledEmails = types.BoolValue(p.SkipMonitorDisabledEmails)
	m.SemconvVersion = types.StringValue(string(p.SemconvVersion))

	// Intentionally do NOT populate the time ranges and retentions from the
	// API response. Those attributes are Optional-only in the schema (no
	// Computed), so the user's HCL is the source of truth. Leaving them
	// untouched here preserves whatever the caller already put in the model
	// and sidesteps CE's SelectByID retention override.
}
