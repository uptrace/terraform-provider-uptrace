package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &IncidentioChannelResource{}
	_ resource.ResourceWithConfigure   = &IncidentioChannelResource{}
	_ resource.ResourceWithImportState = &IncidentioChannelResource{}
)

type IncidentioChannelResource struct {
	client *client.Client
}

func NewIncidentioChannelResource() resource.Resource {
	return &IncidentioChannelResource{}
}

type incidentioChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	URL    types.String `tfsdk:"url"`
	APIKey types.String `tfsdk:"api_key"`
}

var incidentioCRUD = channelCRUD[incidentioChannelModel]{
	TypeName:  "incidentio_channel",
	ProjectID: func(m *incidentioChannelModel) types.String { return m.ProjectID },
	ID:        func(m *incidentioChannelModel) types.String { return m.ID },
	Name:      func(m *incidentioChannelModel) types.String { return m.Name },
	Build:     buildIncidentioRequest,
	Apply:     applyIncidentioToModel,
}

func (r *IncidentioChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incidentio_channel"
}

func (r *IncidentioChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["url"] = schema.StringAttribute{
		Required:    true,
		Description: "Alert Events V2 endpoint URL.",
	}
	attrs["api_key"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "incident.io API key.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace incident.io notification channel.",
		Attributes:  attrs,
	}
}

func (r *IncidentioChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *IncidentioChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan incidentioChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, incidentioCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IncidentioChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state incidentioChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, incidentioCRUD)
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

func (r *IncidentioChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan incidentioChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, incidentioCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IncidentioChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state incidentioChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, incidentioCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *IncidentioChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildIncidentioRequest(ctx context.Context, m *incidentioChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "incidentio"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	body.Params = generated.IncidentioParams{
		URL:    m.URL.ValueString(),
		APIKey: m.APIKey.ValueString(),
	}

	return body, diags
}

func applyIncidentioToModel(ctx context.Context, ch *channelResponse, dst *incidentioChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("incidentio", ch, &diags)
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

	var p generated.IncidentioParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode incidentio params", err.Error())
		return diags
	}
	dst.URL = types.StringValue(p.URL)
	dst.APIKey = tfutil.PreserveIfEmpty(p.APIKey, dst.APIKey)

	return diags
}
