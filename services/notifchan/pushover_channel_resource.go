package notifchan

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

var (
	_ resource.Resource                = &PushoverChannelResource{}
	_ resource.ResourceWithConfigure   = &PushoverChannelResource{}
	_ resource.ResourceWithImportState = &PushoverChannelResource{}
)

type PushoverChannelResource struct {
	client *client.Client
}

func NewPushoverChannelResource() resource.Resource {
	return &PushoverChannelResource{}
}

type pushoverChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	Token    types.String `tfsdk:"token"`
	UserKey  types.String `tfsdk:"user_key"`
	Priority types.Int64  `tfsdk:"priority"`
	Sound    types.String `tfsdk:"sound"`
}

var pushoverCRUD = channelCRUD[pushoverChannelModel]{
	TypeName:  "pushover_channel",
	ProjectID: func(m *pushoverChannelModel) types.String { return m.ProjectID },
	ID:        func(m *pushoverChannelModel) types.String { return m.ID },
	Name:      func(m *pushoverChannelModel) types.String { return m.Name },
	Build:     buildPushoverRequest,
	Apply:     applyPushoverToModel,
}

func (r *PushoverChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pushover_channel"
}

func (r *PushoverChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["token"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Pushover application token.",
	}
	attrs["user_key"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Pushover user key.",
	}
	attrs["priority"] = schema.Int64Attribute{
		Optional:    true,
		Description: "Priority from -2 (lowest) to 2 (emergency).",
		Validators: []validator.Int64{
			int64validator.Between(-2, 2),
		},
	}
	attrs["sound"] = schema.StringAttribute{
		Optional:    true,
		Description: "Notification sound.",
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Pushover notification channel.",
		Attributes:  attrs,
	}
}

func (r *PushoverChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *PushoverChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pushoverChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, pushoverCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PushoverChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pushoverChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, pushoverCRUD)
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

func (r *PushoverChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pushoverChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, pushoverCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PushoverChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pushoverChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, pushoverCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *PushoverChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildPushoverRequest(ctx context.Context, m *pushoverChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "pushover"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	p := api.PushoverParams{
		Token:   m.Token.ValueString(),
		UserKey: m.UserKey.ValueString(),
	}
	// The server requires both keys; an unset one goes as its zero value,
	// which Pushover reads as normal priority and the user's default sound.
	if !m.Priority.IsNull() && !m.Priority.IsUnknown() {
		p.Priority = int(m.Priority.ValueInt64())
	}
	if !m.Sound.IsNull() && !m.Sound.IsUnknown() {
		p.Sound = m.Sound.ValueString()
	}

	body.Params = p

	return body, diags
}

func applyPushoverToModel(ctx context.Context, ch *channelResponse, dst *pushoverChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("pushover", ch, &diags)
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

	var p api.PushoverParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode pushover params", err.Error())
		return diags
	}
	dst.Token = tfutil.PreserveIfEmpty(p.Token, dst.Token)
	dst.UserKey = tfutil.PreserveIfEmpty(p.UserKey, dst.UserKey)
	if p.Priority != 0 || !dst.Priority.IsNull() {
		dst.Priority = types.Int64Value(int64(p.Priority))
	}
	if p.Sound != "" {
		dst.Sound = types.StringValue(p.Sound)
	} else {
		dst.Sound = types.StringNull()
	}

	return diags
}
