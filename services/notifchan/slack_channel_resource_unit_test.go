package notifchan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/api"
)

// --- builder ---

func TestBuildSlackRequest_webhookAuth(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	m := &slackChannelModel{
		Name:       types.StringValue("acc"),
		Priorities: priorities,
		MatchAll:   types.BoolValue(true),
		Condition:  types.StringNull(),
		MonitorIDs: types.SetNull(types.StringType),
		AuthMethod: types.StringValue("webhook"),
		WebhookURL: types.StringValue("https://hooks.slack.com/X"),
		Token:      types.StringNull(),
		Channel:    types.StringNull(),
	}
	body, diags := buildSlackRequest(context.Background(), m)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "slack", body.Type)

	p, ok := body.Params.(api.SlackParams)
	require.True(t, ok)
	require.Equal(t, api.SlackAuthMethod("webhook"), p.AuthMethod)
	require.Equal(t, "https://hooks.slack.com/X", p.WebhookURL)
	// The server requires every key: the unused ones go as empty strings.
	require.Empty(t, p.Token)
	require.Empty(t, p.Channel)
}

func TestBuildSlackRequest_tokenAuth(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	m := &slackChannelModel{
		Name:       types.StringValue("acc"),
		Priorities: priorities,
		MatchAll:   types.BoolValue(true),
		Condition:  types.StringNull(),
		MonitorIDs: types.SetNull(types.StringType),
		AuthMethod: types.StringValue("token"),
		WebhookURL: types.StringNull(),
		Token:      types.StringValue("xoxb-t"),
		Channel:    types.StringValue("#alerts"),
	}
	body, diags := buildSlackRequest(context.Background(), m)
	require.False(t, diags.HasError())

	p, ok := body.Params.(api.SlackParams)
	require.True(t, ok)
	require.Empty(t, p.WebhookURL, "webhook_url goes empty for token auth")
	require.Equal(t, "xoxb-t", p.Token)
	require.Equal(t, "#alerts", p.Channel)
}

// --- apply ---

// Slack's auth_method is echoed by the API; webhook_url/token/channel are
// Sensitive and the backend may return them empty. applySlackToModel must
// preserve the prior state value in that case so the user's input doesn't
// disappear from state on refresh.
func TestApplySlackToModel_preservesRedactedSensitiveValues(t *testing.T) {
	raw, _ := json.Marshal(api.SlackParams{
		AuthMethod: api.SlackAuthMethodToken,
		// WebhookURL/Token/Channel are all redacted as empty strings.
	})
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 7, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "slack",
		Params:                  raw,
	}
	dst := &slackChannelModel{
		Token:   types.StringValue("xoxb-prior"),
		Channel: types.StringValue("#prior"),
	}
	diags := applySlackToModel(context.Background(), ch, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "token", dst.AuthMethod.ValueString())
	require.Equal(t, "xoxb-prior", dst.Token.ValueString(), "redacted token must fall back to prior")
	require.Equal(t, "#prior", dst.Channel.ValueString(), "redacted channel must fall back to prior")
}

// --- ValidateConfig (conditional auth_method) ---

func TestRunSlackAuthValidators_webhookAuth(t *testing.T) {
	tests := []struct {
		name  string
		model slackChannelModel
		ok    bool
	}{
		{
			"webhook auth needs webhook_url",
			slackChannelModel{
				AuthMethod: types.StringValue("webhook"),
				WebhookURL: types.StringNull(),
			},
			false,
		},
		{
			"webhook auth forbids token",
			slackChannelModel{
				AuthMethod: types.StringValue("webhook"),
				WebhookURL: types.StringValue("u"),
				Token:      types.StringValue("t"),
			},
			false,
		},
		{
			"webhook auth ok",
			slackChannelModel{
				AuthMethod: types.StringValue("webhook"),
				WebhookURL: types.StringValue("u"),
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			runSlackAuthValidators(tc.model, &diags)
			require.Equal(t, !tc.ok, diags.HasError())
		})
	}
}

func TestRunSlackAuthValidators_tokenAuth(t *testing.T) {
	tests := []struct {
		name  string
		model slackChannelModel
		ok    bool
	}{
		{
			"token auth needs token+channel",
			slackChannelModel{
				AuthMethod: types.StringValue("token"),
				Token:      types.StringNull(),
				Channel:    types.StringNull(),
			},
			false,
		},
		{
			"token auth forbids webhook_url",
			slackChannelModel{
				AuthMethod: types.StringValue("token"),
				Token:      types.StringValue("t"),
				Channel:    types.StringValue("#c"),
				WebhookURL: types.StringValue("u"),
			},
			false,
		},
		{
			"token auth ok",
			slackChannelModel{
				AuthMethod: types.StringValue("token"),
				Token:      types.StringValue("t"),
				Channel:    types.StringValue("#c"),
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			runSlackAuthValidators(tc.model, &diags)
			require.Equal(t, !tc.ok, diags.HasError())
		})
	}
}

// runSlackAuthValidators mirrors the conditional auth-method block inside
// SlackChannelResource.ValidateConfig. Extracted here only for unit testing;
// the resource ValidateConfig still inlines the same branches because the
// method has to read the plan through req.Config.
func runSlackAuthValidators(cfg slackChannelModel, diags *diag.Diagnostics) {
	if cfg.AuthMethod.IsUnknown() || cfg.AuthMethod.IsNull() {
		return
	}
	switch cfg.AuthMethod.ValueString() {
	case "webhook":
		requireField(cfg.WebhookURL, path.Root("webhook_url"), `auth_method is "webhook"`, diags)
		forbidField(cfg.Token, path.Root("token"), `auth_method is "webhook"`, diags)
		forbidField(cfg.Channel, path.Root("channel"), `auth_method is "webhook"`, diags)
	case "token":
		requireField(cfg.Token, path.Root("token"), `auth_method is "token"`, diags)
		requireField(cfg.Channel, path.Root("channel"), `auth_method is "token"`, diags)
		forbidField(cfg.WebhookURL, path.Root("webhook_url"), `auth_method is "token"`, diags)
	}
}
