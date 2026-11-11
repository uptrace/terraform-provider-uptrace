package notifchan

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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
	_ resource.Resource                = &WebhookChannelResource{}
	_ resource.ResourceWithConfigure   = &WebhookChannelResource{}
	_ resource.ResourceWithImportState = &WebhookChannelResource{}
)

type WebhookChannelResource struct {
	client *client.Client
}

func NewWebhookChannelResource() resource.Resource {
	return &WebhookChannelResource{}
}

type webhookChannelModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	MatchAll   types.Bool   `tfsdk:"match_all"`
	Condition  types.String `tfsdk:"condition"`
	Priorities types.List   `tfsdk:"priorities"`
	MonitorIDs types.Set    `tfsdk:"monitor_ids"`

	URL     types.String `tfsdk:"url"`
	Payload types.String `tfsdk:"payload"`
}

var webhookCRUD = channelCRUD[webhookChannelModel]{
	TypeName:  "webhook_channel",
	ProjectID: func(m *webhookChannelModel) types.String { return m.ProjectID },
	ID:        func(m *webhookChannelModel) types.String { return m.ID },
	Name:      func(m *webhookChannelModel) types.String { return m.Name },
	Build:     buildWebhookRequest,
	Apply:     applyWebhookToModel,
}

func (r *WebhookChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_channel"
}

func (r *WebhookChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sharedAttributes()
	attrs["url"] = schema.StringAttribute{
		Required:    true,
		Sensitive:   true,
		Description: "Webhook URL.",
	}
	attrs["payload"] = schema.StringAttribute{
		Optional:    true,
		Description: "Optional JSON object sent as the webhook payload. Use jsonencode() to construct.",
		Validators: []validator.String{
			jsonObjectValidator{},
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages an Uptrace Webhook notification channel.",
		Attributes:  attrs,
	}
}

func (r *WebhookChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = tfutil.FromProviderData[client.Client](req.ProviderData, &resp.Diagnostics)
}

func (r *WebhookChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelCreate(ctx, r.client, &plan, webhookCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebhookChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	remove, diags := channelRead(ctx, r.client, &state, webhookCRUD)
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

func (r *WebhookChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelUpdate(ctx, r.client, &plan, webhookCRUD)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebhookChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(channelDelete(ctx, r.client, &state, webhookCRUD)...)
}

// ImportState accepts "<project_id>:<channel_id>".
func (r *WebhookChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChannel(ctx, req, resp)
}

func buildWebhookRequest(ctx context.Context, m *webhookChannelModel) (*channelRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := &channelRequest{Type: "webhook"}
	diags.Append(buildSharedRequest(ctx, sharedFields{
		Name: &m.Name, MatchAll: &m.MatchAll, Condition: &m.Condition,
		Priorities: &m.Priorities, MonitorIDs: &m.MonitorIDs,
	}, body)...)
	if diags.HasError() {
		return nil, diags
	}

	p := api.WebhookParams{URL: m.URL.ValueString()}
	if !m.Payload.IsNull() && !m.Payload.IsUnknown() {
		var payload map[string]any
		if err := json.Unmarshal([]byte(m.Payload.ValueString()), &payload); err != nil {
			diags.AddError("invalid webhook payload", err.Error())
			return nil, diags
		}
		p.Payload = payload
	}

	body.Params = p

	return body, diags
}

func applyWebhookToModel(ctx context.Context, ch *channelResponse, dst *webhookChannelModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rejectChannelType("webhook", ch, &diags)
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

	var p api.WebhookParams
	if err := json.Unmarshal(ch.Params, &p); err != nil {
		diags.AddWarning("failed to decode webhook params", err.Error())
		return diags
	}
	dst.URL = tfutil.PreserveIfEmpty(p.URL, dst.URL)
	// The contract states no type for the payload, so it decodes as any: an
	// object is a map, and null is nil.
	payload, _ := p.Payload.(map[string]any)
	if payloadEquivalent(dst.Payload, payload) {
		// The configured spelling stays the source of truth while the server
		// echoes the same value (an empty object included): a re-encoded
		// string would differ from the plan and fail the apply.
		return diags
	}
	if len(payload) > 0 {
		buf, err := json.Marshal(payload)
		if err != nil {
			diags.AddWarning(
				"failed to encode webhook payload",
				fmt.Sprintf("could not marshal webhook payload to JSON: %s", err.Error()),
			)
		} else {
			dst.Payload = types.StringValue(string(buf))
		}
	} else {
		dst.Payload = types.StringNull()
	}

	return diags
}

// payloadEquivalent reports whether the configured payload string decodes to
// the value the server returned. A null or unknown string is equivalent to
// nothing, so an import takes the server's value.
func payloadEquivalent(configured types.String, payload map[string]any) bool {
	if configured.IsNull() || configured.IsUnknown() {
		return false
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(configured.ValueString()), &decoded); err != nil {
		return false
	}
	if len(decoded) == 0 && len(payload) == 0 {
		return true
	}
	return reflect.DeepEqual(decoded, payload)
}
