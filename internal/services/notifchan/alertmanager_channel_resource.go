package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var (
	_ resource.Resource                   = &AlertmanagerChannelResource{}
	_ resource.ResourceWithConfigure      = &AlertmanagerChannelResource{}
	_ resource.ResourceWithImportState    = &AlertmanagerChannelResource{}
	_ resource.ResourceWithValidateConfig = &AlertmanagerChannelResource{}
)

var alertmanagerAuthMethods = []string{"none", "basic_auth", "bearer"}

type AlertmanagerChannelResource struct {
	client *client.Client
}

func NewAlertmanagerChannelResource() resource.Resource {
	return &AlertmanagerChannelResource{}
}

type alertmanagerChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	URL        types.String `tfsdk:"url"`
	AuthMethod types.String `tfsdk:"auth_method"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	Token      types.String `tfsdk:"token"`
}

var alertmanagerCRUD = channelCRUD[alertmanagerChannelModel]{
	TypeName:  "alertmanager_channel",
	ProjectID: func(m *alertmanagerChannelModel) types.String { return m.ProjectID },
	ID:        func(m *alertmanagerChannelModel) types.String { return m.ID },
	Name:      func(m *alertmanagerChannelModel) types.String { return m.Name },
	Build:     buildAlertmanagerRequest,
	Apply:     applyAlertmanagerToModel,
}

func (r *AlertmanagerChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alertmanager_channel"
}

func (r *AlertmanagerChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["url"] = schema.StringAttribute{
		Required:    true,
		Description: "Alertmanager API endpoint URL.",
	}
	attrs["auth_method"] = schema.StringAttribute{
		Optional:    true,
		Description: "Authentication method: none, basic_auth, or bearer.",
		Validators: []validator.String{
			stringvalidator.OneOf(alertmanagerAuthMethods...),
		},
	}
	attrs["username"] = schema.StringAttribute{
		Optional:    true,
		Description: "Username for basic_auth.",
	}
	attrs["password"] = schema.StringAttribute{
		Optional:    true,
		Sensitive:   true,
		Description: "Password for basic_auth.",
	}
	attrs["token"] = schema.StringAttribute{
		Optional:    true,
		Sensitive:   true,
		Description: "Token for bearer auth.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Alertmanager notification channel.",
		Attributes:  attrs,
	}
}

func (r *AlertmanagerChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *AlertmanagerChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg alertmanagerChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateMatchAllRule(cfg.MatchAll, cfg.MonitorIDs, &resp.Diagnostics)
	runAlertmanagerAuthValidators(&cfg, &resp.Diagnostics)
}

func runAlertmanagerAuthValidators(cfg *alertmanagerChannelModel, diags *diag.Diagnostics) {
	if cfg.AuthMethod.IsUnknown() {
		return
	}
	if cfg.AuthMethod.IsNull() {
		if fieldIsSet(cfg.Username) || fieldIsSet(cfg.Password) || fieldIsSet(cfg.Token) {
			diags.AddAttributeError(
				path.Root("auth_method"),
				"auth_method is required",
				"auth_method must be set when username, password, or token is set.",
			)
		}
		return
	}

	switch cfg.AuthMethod.ValueString() {
	case "basic_auth":
		requireField(cfg.Username, path.Root("username"), `auth_method is "basic_auth"`, diags)
		requireField(cfg.Password, path.Root("password"), `auth_method is "basic_auth"`, diags)
		forbidField(cfg.Token, path.Root("token"), `auth_method is "basic_auth"`, diags)
	case "bearer":
		requireField(cfg.Token, path.Root("token"), `auth_method is "bearer"`, diags)
		forbidField(cfg.Username, path.Root("username"), `auth_method is "bearer"`, diags)
		forbidField(cfg.Password, path.Root("password"), `auth_method is "bearer"`, diags)
	case "none":
		forbidField(cfg.Username, path.Root("username"), `auth_method is "none"`, diags)
		forbidField(cfg.Password, path.Root("password"), `auth_method is "none"`, diags)
		forbidField(cfg.Token, path.Root("token"), `auth_method is "none"`, diags)
	}
}

func (r *AlertmanagerChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertmanagerChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, alertmanagerCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlertmanagerChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertmanagerChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, alertmanagerCRUD)
	resp.Diagnostics.Append(diags...)
	if remove {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AlertmanagerChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertmanagerChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, alertmanagerCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlertmanagerChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertmanagerChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, alertmanagerCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *AlertmanagerChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildAlertmanagerRequest(ctx context.Context, m *alertmanagerChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "alertmanager"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	p := generated.AlertmanagerParams{URL: m.URL.ValueString()}
	if !m.AuthMethod.IsNull() && !m.AuthMethod.IsUnknown() {
		v := generated.HTTPAuthMethod(m.AuthMethod.ValueString())
		p.AuthMethod = &v
	}
	p.Username = valueStringToPtr(m.Username)
	p.Password = valueStringToPtr(m.Password)
	p.Token = valueStringToPtr(m.Token)

	body.Params = p

	return body, diags
}

func applyAlertmanagerToModel(ctx context.Context, ch *channelResponse, dst *alertmanagerChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("alertmanager", ch, &diags)
	if diags.HasError() {
		return diags
	}

	diags.Append(applyShared(ctx, ch, sharedFields{
		ID: &dst.ID, Name: &dst.Name, Status: &dst.Status,
		MatchAll: &dst.MatchAll, Condition: &dst.Condition,
		Priorities: &dst.Priorities, MonitorIDs: &dst.MonitorIDs,
	})...)
	if diags.HasError() {
		return diags
	}

	var p generated.AlertmanagerParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode alertmanager params", err.Error())
		return diags
	}
	dst.URL = types.StringValue(p.URL)
	dst.AuthMethod = tfutil.EnumToValue(p.AuthMethod)
	dst.Username = tfutil.StringFromPtr(p.Username)
	dst.Password = tfutil.PreserveIfEmptyPtr(p.Password, dst.Password)
	dst.Token = tfutil.PreserveIfEmptyPtr(p.Token, dst.Token)

	return diags
}
