package notifchan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/internal/generated"
)

func idSet(ids ...string) types.Set {
	vals := make([]attr.Value, len(ids))
	for i, id := range ids {
		vals[i] = types.StringValue(id)
	}
	return types.SetValueMust(types.StringType, vals)
}

func emailModel(everyone bool, teamIDs, userIDs types.Set) *emailChannelModel {
	priorities, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"high"})
	return &emailChannelModel{
		Name:       types.StringValue("acc"),
		Priorities: priorities,
		MatchAll:   types.BoolValue(true),
		Condition:  types.StringNull(),
		MonitorIDs: types.SetNull(types.StringType),
		Everyone:   types.BoolValue(everyone),
		TeamIDs:    teamIDs,
		UserIDs:    userIDs,
	}
}

func TestBuildEmailRequest_everyoneSendsEmptyLists(t *testing.T) {
	body, diags := buildEmailRequest(context.Background(), emailModel(true, idSet(), idSet()))
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)
	require.Equal(t, "email", body.Type)

	// The spec requires both lists: they must go as [], not null.
	b, err := json.Marshal(body.Params)
	require.NoError(t, err)
	require.JSONEq(t, `{"everyone": true, "teamIds": [], "userIds": []}`, string(b))
}

func TestBuildEmailRequest_lists(t *testing.T) {
	body, diags := buildEmailRequest(context.Background(), emailModel(false, idSet("7"), idSet("3", "5")))
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)

	p, ok := body.Params.(generated.EmailParams)
	require.True(t, ok)
	require.False(t, p.Everyone)
	require.Equal(t, []uint64{7}, p.TeamIds)
	require.ElementsMatch(t, []uint64{3, 5}, p.UserIds)
}

func TestBuildEmailRequest_rejectsNonNumericID(t *testing.T) {
	_, diags := buildEmailRequest(context.Background(), emailModel(false, idSet("x"), idSet()))
	require.True(t, diags.HasError())
}

func TestApplyEmailToModel_roundTripsAudience(t *testing.T) {
	raw, _ := json.Marshal(generated.EmailParams{TeamIds: []uint64{7}, UserIds: []uint64{}})
	ch := &channelResponse{
		BaseNotificationChannel: generated.BaseNotificationChannel{ID: 9, Name: "c", Status: generated.NotificationChannelStatusDelivering},
		Type:                    "email",
		Params:                  raw,
	}
	dst := emailModel(true, types.SetNull(types.StringType), types.SetNull(types.StringType))
	diags := applyEmailToModel(context.Background(), ch, dst)
	require.False(t, diags.HasError(), "unexpected errors: %v", diags)

	require.False(t, dst.Everyone.ValueBool())
	require.True(t, dst.TeamIDs.Equal(idSet("7")))
	// An empty list reads back as an empty set, which equals the schema default.
	require.True(t, dst.UserIDs.Equal(idSet()))
}

func TestApplyEmailToModel_rejectsOtherType(t *testing.T) {
	ch := &channelResponse{Type: "telegram", Params: json.RawMessage(`{"chatId": 1}`)}
	diags := applyEmailToModel(context.Background(), ch, emailModel(true, idSet(), idSet()))
	require.True(t, diags.HasError())
}

func TestValidateEmailAudience(t *testing.T) {
	tests := []struct {
		name     string
		everyone types.Bool
		teamIDs  types.Set
		userIDs  types.Set
		wantErr  bool
	}{
		{"everyone", types.BoolValue(true), idSet(), idSet(), false},
		{"teams", types.BoolValue(false), idSet("1"), idSet(), false},
		{"users", types.BoolValue(false), idSet(), idSet("2"), false},
		{"everyone and a list", types.BoolValue(true), idSet(), idSet("2"), true},
		{"no one", types.BoolValue(false), idSet(), idSet(), true},
		{"unknown list", types.BoolValue(false), types.SetUnknown(types.StringType), idSet(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEmailAudience(tt.everyone, tt.teamIDs, tt.userIDs, &diags)
			require.Equal(t, tt.wantErr, diags.HasError(), "diags: %v", diags)
		})
	}
}
