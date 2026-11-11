package notifchan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/api"
)

// --- builder ---

func TestBuildWebhookRequest_withPayload(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	m := &webhookChannelModel{
		Name:       types.StringValue("acc"),
		Priorities: priorities,
		MatchAll:   types.BoolValue(true),
		Condition:  types.StringNull(),
		MonitorIDs: types.SetNull(types.StringType),
		URL:        types.StringValue("https://example.com/hook"),
		Payload:    types.StringValue(`{"source":"uptrace","severity":"high"}`),
	}
	body, diags := buildWebhookRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)

	p, ok := body.Params.(api.WebhookParams)
	require.True(t, ok)
	require.Equal(t, "https://example.com/hook", p.URL)
	require.Equal(t, map[string]any{"source": "uptrace", "severity": "high"}, p.Payload)
}

func TestBuildWebhookRequest_invalidPayload(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	m := &webhookChannelModel{
		Name:       types.StringValue("acc"),
		Priorities: priorities,
		MatchAll:   types.BoolValue(true),
		Condition:  types.StringNull(),
		MonitorIDs: types.SetNull(types.StringType),
		URL:        types.StringValue("https://example.com/hook"),
		Payload:    types.StringValue(`not-json`),
	}
	_, diags := buildWebhookRequest(context.Background(), m)
	require.True(t, diags.HasError(), "invalid JSON payload must emit a diagnostic")
}

// --- apply ---

// When the user did not set a payload, state must stay null even if the
// backend echoes an empty object — otherwise plan-after-apply produces
// "inconsistent result" errors.
func TestApplyWebhookToModel_emptyPayloadStaysNull(t *testing.T) {
	raw, _ := json.Marshal(api.WebhookParams{
		URL:     "https://example.com/hook",
		Payload: map[string]any{},
	})
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 7, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "webhook",
		Params:                  raw,
	}
	dst := &webhookChannelModel{
		URL:     types.StringValue("https://example.com/hook"),
		Payload: types.StringNull(),
	}
	diags := applyWebhookToModel(context.Background(), ch, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.True(t, dst.Payload.IsNull(), "empty server payload must leave state null")
}

func TestApplyWebhookToModel_payloadRoundTrip(t *testing.T) {
	raw, _ := json.Marshal(api.WebhookParams{
		URL:     "https://example.com/hook",
		Payload: map[string]any{"source": "uptrace"},
	})
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 7, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "webhook",
		Params:                  raw,
	}
	dst := &webhookChannelModel{URL: types.StringValue("https://example.com/hook")}
	diags := applyWebhookToModel(context.Background(), ch, dst)
	require.False(t, diags.HasError())
	require.False(t, dst.Payload.IsNull())
	// Decoded JSON must be a valid object containing the round-tripped key.
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(dst.Payload.ValueString()), &out))
	require.Equal(t, "uptrace", out["source"])
}

// The server stores the payload as JSON and echoes it, so the configured
// string stays in state while the two decode to the same value, an empty
// object included; a plan then shows what the configuration says, as the
// configured spelling of a monitor query does. A different value is taken.
func TestApplyWebhookToModel_equivalentPayloadKeepsTheConfiguredString(t *testing.T) {
	respond := func(t *testing.T, payload map[string]any) *channelResponse {
		t.Helper()
		raw, err := json.Marshal(api.WebhookParams{URL: "https://example.com/hook", Payload: payload})
		require.NoError(t, err)
		return &channelResponse{
			BaseNotificationChannel: api.BaseNotificationChannel{ID: 7, Name: "c", Status: api.NotificationChannelStatusDelivering},
			Type:                    "webhook",
			Params:                  raw,
		}
	}
	apply := func(t *testing.T, configured string, payload map[string]any) types.String {
		t.Helper()
		dst := &webhookChannelModel{URL: types.StringValue("https://example.com/hook"), Payload: types.StringValue(configured)}
		diags := applyWebhookToModel(context.Background(), respond(t, payload), dst)
		require.False(t, diags.HasError(), "unexpected errors: %v", diags)
		return dst.Payload
	}

	t.Run("an empty object stays the configured string", func(t *testing.T) {
		require.Equal(t, types.StringValue("{}"), apply(t, "{}", map[string]any{}))
	})
	t.Run("a formatted object stays the configured string", func(t *testing.T) {
		configured := "{\n  \"source\": \"uptrace\"\n}"
		require.Equal(t, types.StringValue(configured), apply(t, configured, map[string]any{"source": "uptrace"}))
	})
	t.Run("a different object replaces the configured string", func(t *testing.T) {
		got := apply(t, `{"source":"uptrace"}`, map[string]any{"source": "other"})
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(got.ValueString()), &out))
		require.Equal(t, "other", out["source"])
	})
}

// --- jsonObjectValidator ---

func TestJSONObjectValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null ok", types.StringNull(), false},
		{"unknown ok", types.StringUnknown(), false},
		{"valid object ok", types.StringValue(`{"k":"v"}`), false},
		{"array rejected", types.StringValue(`[1,2]`), true},
		{"scalar rejected", types.StringValue(`"str"`), true},
		{"malformed rejected", types.StringValue(`{not json`), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("payload"),
				ConfigValue: tc.value,
			}
			resp := &validator.StringResponse{}
			jsonObjectValidator{}.ValidateString(context.Background(), req, resp)
			require.Equal(t, tc.wantErr, resp.Diagnostics.HasError())
		})
	}
}
