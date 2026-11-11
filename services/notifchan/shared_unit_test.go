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

type testChannelModel struct {
	projectID types.String
	id        types.String
	name      types.String
}

// --- applyShared ---

func TestApplyShared_monitorIDsExplicitEmptyPreserved(t *testing.T) {
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 42, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "slack",
	}
	priorEmpty, _ := types.SetValueFrom(context.Background(), types.StringType, []string{})
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = priorEmpty
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.False(t, monitorIDs.IsNull(), "explicit empty set must round-trip")
	require.Equal(t, 0, len(monitorIDs.Elements()))
}

func TestApplyShared_monitorIDsNilBecomesNull(t *testing.T) {
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 42, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "slack",
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, monitorIDs.IsNull(), "nil prior + empty API → SetNull")
}

func TestApplyShared_unsetMonitorIDsIgnoreLinksFromMonitors(t *testing.T) {
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 42, Name: "c", MonitorIDs: []uint64{7}},
		Type:                    "slack",
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, monitorIDs.IsNull(), "a link a monitor declared must not show as drift on the channel")
}

func TestApplyShared_conditionNilBecomesNull(t *testing.T) {
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 42, Name: "c", Status: api.NotificationChannelStatusDelivering},
		Type:                    "slack",
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, condition.IsNull())
}

func TestApplyShared_matchAllFollowsAPI(t *testing.T) {
	ch := &channelResponse{
		BaseNotificationChannel: api.BaseNotificationChannel{ID: 42, Name: "c", Status: api.NotificationChannelStatusDelivering, MatchAll: true},
		Type:                    "slack",
	}
	var (
		id, name, status, condition types.String
		matchAll                    types.Bool
		priorities                  types.List
		monitorIDs                  = types.SetNull(types.StringType)
	)
	diags := applyShared(context.Background(), ch, sharedFields{
		ID: &id, Name: &name, Status: &status, MatchAll: &matchAll,
		Condition: &condition, Priorities: &priorities, MonitorIDs: &monitorIDs,
	})
	require.False(t, diags.HasError())
	require.True(t, matchAll.ValueBool())
}

// --- buildSharedRequest ---

func TestBuildSharedRequest_monitorIDsInvalidIDEmitsDiagnostic(t *testing.T) {
	badIDs, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"12", "not-a-number"})
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	name := types.StringValue("x")
	matchAll := types.BoolValue(true)
	condition := types.StringNull()

	body := &channelRequest{}
	diags := buildSharedRequest(context.Background(), sharedFields{
		Name: &name, MatchAll: &matchAll, Condition: &condition,
		Priorities: &priorities, MonitorIDs: &badIDs,
	}, body)
	require.True(t, diags.HasError(), "non-numeric monitor_id must emit a diagnostic")
	require.Len(t, diags, 1)
	require.Equal(t, "invalid monitor_id", diags[0].Summary())
	attrDiag, ok := diags[0].(diag.DiagnosticWithPath)
	require.True(t, ok, "invalid monitor_id diagnostic must be attributed to monitor_ids")
	require.True(t, path.Root("monitor_ids").Equal(attrDiag.Path()))
}

func TestBuildSharedRequest_prioritiesRoundTrip(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high", "medium"})
	monitorIDs := types.SetNull(types.StringType)
	name := types.StringValue("x")
	matchAll := types.BoolValue(true)
	condition := types.StringNull()

	body := &channelRequest{}
	diags := buildSharedRequest(context.Background(), sharedFields{
		Name: &name, MatchAll: &matchAll, Condition: &condition,
		Priorities: &priorities, MonitorIDs: &monitorIDs,
	}, body)
	require.False(t, diags.HasError())
	require.Equal(t, []api.AlertPriority{
		"high", "medium",
	}, body.Priorities)
	require.Nil(t, body.MonitorIDs, "unset monitor_ids omits the key, which keeps the links monitors declared")
}

// The server keeps the stored condition when the key is absent and clears it
// on an empty string, so a null plan must send the empty string: a condition
// removed from the configuration is otherwise kept, and the apply ends
// inconsistent.
func TestBuildSharedRequest_nullConditionClears(t *testing.T) {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	monitorIDs := types.SetNull(types.StringType)
	name := types.StringValue("x")
	matchAll := types.BoolValue(true)
	condition := types.StringNull()

	body := &channelRequest{}
	diags := buildSharedRequest(context.Background(), sharedFields{
		Name: &name, MatchAll: &matchAll, Condition: &condition,
		Priorities: &priorities, MonitorIDs: &monitorIDs,
	}, body)
	require.False(t, diags.HasError())
	require.NotNil(t, body.Condition, "a null condition sends the key")
	require.Equal(t, "", *body.Condition, "an empty condition clears the stored one")
}

// --- channelRequest / channelResponse ---

func TestChannelRequest_toAPIKeepsEveryKey(t *testing.T) {
	condition := ""
	body := &channelRequest{
		Name:       "tg",
		MonitorIDs: &[]uint64{},
		Condition:  &condition,
		Priorities: []api.AlertPriority{"high"},
		Type:       "telegram",
		Params:     api.TelegramParams{ChatID: -100},
	}

	req, err := body.toAPI()
	require.NoError(t, err)

	b, err := json.Marshal(req)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "telegram", got["type"])
	require.Equal(t, "tg", got["name"])
	require.Equal(t, []any{}, got["monitorIds"], "an empty list must be sent, not omitted")
	require.Equal(t, "", got["condition"], "an empty condition must be sent, not omitted")
	require.Equal(t, map[string]any{"chatId": float64(-100)}, got["params"])
}

func TestChannelFromAPI_decodesBaseTypeAndParams(t *testing.T) {
	var ch api.NotificationChannel
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": 9, "projectId": 1, "name": "tg", "status": "delivering", "pausedUntil": 0,
		"error": "", "matchAll": false, "condition": "", "priorities": ["high"],
		"monitorIds": [3], "type": "telegram", "params": {"chatId": -100}
	}`), &ch))

	resp, err := channelFromAPI(&ch)
	require.NoError(t, err)
	require.Equal(t, uint64(9), resp.ID)
	require.Equal(t, "telegram", resp.Type)
	require.Equal(t, []uint64{3}, resp.MonitorIDs)

	var p api.TelegramParams
	require.NoError(t, json.Unmarshal(resp.Params, &p))
	require.Equal(t, int64(-100), p.ChatID)
}

// --- rejectChannelType ---

func TestRejectChannelType_mismatch(t *testing.T) {
	ch := &channelResponse{BaseNotificationChannel: api.BaseNotificationChannel{ID: 1}, Type: "slack"}
	var diags diag.Diagnostics
	rejectChannelType("webhook", ch, &diags)
	require.True(t, diags.HasError())
}

func TestRejectChannelType_match(t *testing.T) {
	ch := &channelResponse{BaseNotificationChannel: api.BaseNotificationChannel{ID: 1}, Type: "slack"}
	var diags diag.Diagnostics
	rejectChannelType("slack", ch, &diags)
	require.False(t, diags.HasError())
}

func testChannelCRUD(t *testing.T) channelCRUD[testChannelModel] {
	t.Helper()

	return channelCRUD[testChannelModel]{
		TypeName: "test_channel",
		ProjectID: func(m *testChannelModel) types.String {
			return m.projectID
		},
		ID: func(m *testChannelModel) types.String {
			return m.id
		},
		Name: func(m *testChannelModel) types.String {
			return m.name
		},
		Build: func(context.Context, *testChannelModel) (*channelRequest, diag.Diagnostics) {
			t.Fatalf("Build should not be called when project_id parsing fails")
			return nil, nil
		},
		Apply: func(context.Context, *channelResponse, *testChannelModel) diag.Diagnostics {
			t.Fatalf("Apply should not be called when project_id parsing fails")
			return nil
		},
	}
}

func TestChannelCreate_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelCreate(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	removeResource, diags := channelRead(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.False(t, removeResource)
	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelUpdate(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}

func TestChannelDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	diags := channelDelete(context.Background(), nil, &testChannelModel{
		projectID: types.StringValue("4294967296"),
		id:        types.StringValue("1"),
		name:      types.StringValue("test"),
	}, testChannelCRUD(t))

	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Summary(), "invalid project_id")
}
