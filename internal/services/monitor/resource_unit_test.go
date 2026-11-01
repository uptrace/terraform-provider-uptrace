package monitor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/internal/generated"
)

// serverParams is a params object per kind as the server returns it.
var serverParams = map[string]string{
	"threshold": `{
		"metrics": [{"name": "go_goroutine_count", "alias": "$g"}],
		"query": "sum($g{}) | group by service_name",
		"column": {"name": "sum($g{})", "unit": ""},
		"resolution": 60000, "numEvalPoints": 5, "windowRule": "at_all_times",
		"minBreachPoints": 0, "absentPoints": "alert", "timeOffset": 0,
		"minValue": 1, "maxValue": null,
		"recovery": {"minValue": null, "maxValue": null}
	}`,
	"anomaly": `{
		"metrics": [{"name": "uptrace_tracing_spans", "alias": "$spans"}],
		"query": "p50($spans{})", "column": {"name": "p50($spans{})", "unit": "milliseconds"},
		"resolution": 60000, "numEvalPoints": 1, "windowRule": "at_all_times",
		"minBreachPoints": 0, "absentPoints": "ignore", "timeOffset": 0,
		"preset": "general", "minVolume": 0, "minChange": 0,
		"sensitivity": "medium", "direction": "both", "seasonality": "auto"
	}`,
	"outlier": `{
		"metrics": [{"name": "system_cpu_load_average_1m", "alias": "$load"}],
		"query": "avg($load{}) | group by host_name", "column": {"name": "avg($load{})", "unit": ""},
		"resolution": 60000, "numEvalPoints": 1, "windowRule": "at_all_times",
		"minBreachPoints": 0, "absentPoints": "ignore", "timeOffset": 0,
		"minPeers": 3, "minChange": 0, "sensitivity": "medium", "direction": "both",
		"keepFiringFor": 300000
	}`,
	"forecast": `{
		"metrics": [{"name": "system_filesystem_usage", "alias": "$fs"}],
		"query": "sum($fs{state=\"used\"}) / sum($fs{}) as util", "column": {"name": "util", "unit": ""},
		"resolution": 60000, "numEvalPoints": 1, "windowRule": "at_all_times",
		"minBreachPoints": 0, "absentPoints": "ignore", "timeOffset": 0,
		"minValue": null, "maxValue": 0.96, "seasonality": "auto", "horizon": 259200000
	}`,
	"error": `{
		"metrics": [{"name": "uptrace_tracing_logs", "alias": "$logs"}],
		"query": "sum($logs{}) | group by _group_id"
	}`,
	"slo": `{
		"metrics": [{"name": "uptrace_tracing_spans", "alias": "$spans"}],
		"sli": {"type": "event", "goodQuery": "histogram_count($spans{_status_code=\"ok\"})", "totalQuery": "histogram_count($spans{})"},
		"grouping": ["service_name"],
		"resolution": 60000, "target": 0.999, "windowKind": "rolling", "rollingWindow": 2592000000,
		"rules": [
			{"name": "fast", "priority": "high", "type": "burn_rate",
			 "config": {"longWindow": 3600000, "shortWindow": 300000, "factor": 14.4, "minTotal": 0}},
			{"name": "spent", "priority": "medium", "type": "budget_consumed",
			 "config": {"consumedBound": 0.5, "minTotal": 10}}
		]
	}`,
}

func kindByName(t *testing.T, name string) *kind {
	t.Helper()
	for _, k := range kinds {
		if k.name == name {
			return k
		}
	}
	t.Fatalf("no kind %q", name)
	return nil
}

func paramsFromServer(t *testing.T, k *kind) types.Object {
	t.Helper()
	obj, diags := k.fromAPI(context.Background(), json.RawMessage(serverParams[k.name]), types.ObjectNull(k.paramsAttrTypes()))
	require.False(t, diags.HasError(), "fromAPI: %v", diags)
	return obj
}

// generatedRoundTrip decodes body into the kind's generated request, as
// sendAs does, and encodes it again: what the client sends on the wire.
func generatedRoundTrip(t *testing.T, kindName string, body []byte) map[string]any {
	t.Helper()
	var req any
	switch kindName {
	case "threshold":
		req = new(generated.ThresholdMonitorRequest)
	case "anomaly":
		req = new(generated.AnomalyMonitorRequest)
	case "outlier":
		req = new(generated.OutlierMonitorRequest)
	case "forecast":
		req = new(generated.ForecastMonitorRequest)
	case "error":
		req = new(generated.ErrorMonitorRequest)
	case "slo":
		req = new(generated.SloMonitorRequest)
	}
	require.NoError(t, json.Unmarshal(body, req))
	wire, err := json.Marshal(req)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(wire, &out))
	return out
}

func testModel(t *testing.T, k *kind, params types.Object) *monitorModel {
	t.Helper()
	ri, diags := types.ObjectValue(repeatIntervalAttrTypes, map[string]attr.Value{
		"strategy": types.StringValue("fixed"),
		"interval": types.StringValue("1h"),
		"min":      types.StringNull(),
		"max":      types.StringNull(),
	})
	require.False(t, diags.HasError())
	return &monitorModel{
		ID:               types.StringNull(),
		ProjectID:        types.StringValue("21"),
		Name:             types.StringValue("Dev: " + k.name),
		Description:      types.StringNull(),
		ChannelIDs:       types.SetNull(types.StringType),
		TrendAggFunc:     types.StringNull(),
		TrendSensitivity: types.StringNull(),
		RepeatInterval:   ri,
		Status:           types.StringNull(),
		Params:           params,
	}
}

// TestRequestBody_survivesGeneratedRequest checks, for every kind, that the
// body the resource encodes passes through the generated request type with
// no key lost or added: the server refuses an unknown key and a missing
// required one.
func TestRequestBody_survivesGeneratedRequest(t *testing.T) {
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			r := &monitorResource{kind: k}
			body, diags := r.requestBody(context.Background(), testModel(t, k, paramsFromServer(t, k)))
			require.False(t, diags.HasError(), "requestBody: %v", diags)

			var sent map[string]any
			require.NoError(t, json.Unmarshal(body, &sent))
			require.Equal(t, sent, generatedRoundTrip(t, k.name, body))

			require.Contains(t, sent, "channelIds")
			require.Nil(t, sent["channelIds"], "an unset channel_ids sends null, which keeps the stored links")
			require.Equal(t, map[string]any{"strategy": "fixed", "interval": float64(3_600_000)}, sent["repeatInterval"])
			require.NotContains(t, sent, "type", "the route states the type; the body must not")
		})
	}
}

func TestRequestBody_emptyChannelIDsDetach(t *testing.T) {
	k := kindByName(t, "threshold")
	m := testModel(t, k, paramsFromServer(t, k))
	m.ChannelIDs = types.SetValueMust(types.StringType, nil)

	body, diags := (&monitorResource{kind: k}).requestBody(context.Background(), m)
	require.False(t, diags.HasError())
	var sent map[string]any
	require.NoError(t, json.Unmarshal(body, &sent))
	require.Equal(t, []any{}, sent["channelIds"], "channel_ids = [] must send [] so the server detaches every channel")
	require.Equal(t, []any{}, generatedRoundTrip(t, k.name, body)["channelIds"])
}

func TestThresholdParams_recoveryAndNullBounds(t *testing.T) {
	k := kindByName(t, "threshold")
	params, diags := k.toAPI(context.Background(), paramsFromServer(t, k))
	require.False(t, diags.HasError())

	b, err := json.Marshal(params)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	require.Equal(t, float64(1), got["minValue"])
	require.NotContains(t, got, "maxValue", "an unset bound is omitted")
	require.NotContains(t, got, "recovery", "a recovery with no bound is not configured")
	require.Equal(t, float64(60_000), got["resolution"])
}

func TestThresholdParams_recoverySendsBothKeys(t *testing.T) {
	k := kindByName(t, "threshold")
	raw := `{"metrics": [{"name": "m", "alias": "$m"}], "query": "avg($m)",
		"maxValue": 0.9, "recovery": {"minValue": null, "maxValue": 0.8}}`
	obj, diags := k.fromAPI(context.Background(), json.RawMessage(raw), types.ObjectNull(k.paramsAttrTypes()))
	require.False(t, diags.HasError())

	params, diags := k.toAPI(context.Background(), obj)
	require.False(t, diags.HasError())
	b, err := json.Marshal(params)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, map[string]any{"minValue": nil, "maxValue": 0.8}, got["recovery"],
		"the server refuses a recovery that omits either bound")
}

func TestSLOParams_rulesRoundTrip(t *testing.T) {
	k := kindByName(t, "slo")
	params, diags := k.toAPI(context.Background(), paramsFromServer(t, k))
	require.False(t, diags.HasError())

	b, err := json.Marshal(params)
	require.NoError(t, err)
	var got, want map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.NoError(t, json.Unmarshal([]byte(serverParams["slo"]), &want))
	require.Equal(t, want, got)
}

func TestApply_keepsConfiguredSpellings(t *testing.T) {
	k := kindByName(t, "threshold")
	r := &monitorResource{kind: k}
	ctx := context.Background()

	m := testModel(t, k, paramsFromServer(t, k))
	// The config says "60s" and an unnormalized query; the server answers
	// 60000 ms and the normalized query.
	var p thresholdParamsModel
	require.False(t, m.Params.As(ctx, &p, basetypesObjectAsOptions()).HasError())
	p.Resolution = types.StringValue("60s")
	p.Query = types.StringValue("sum($g) | group by service_name")
	var d diag.Diagnostics
	m.Params = paramsToObject(ctx, k, &p, &d)
	require.False(t, d.HasError())

	server := map[string]any{
		"id": 7, "type": "threshold", "name": "Dev: threshold", "description": "",
		"status": "active", "channelIds": []any{}, "trendAggFunc": "sum", "trendSensitivity": "medium",
		"repeatInterval": map[string]any{"strategy": "fixed", "interval": 3_600_000},
		"params":         json.RawMessage(serverParams["threshold"]),
	}
	diags := r.apply(ctx, server, m)
	require.False(t, diags.HasError(), "apply: %v", diags)

	require.Equal(t, "7", m.ID.ValueString())
	require.True(t, m.ChannelIDs.IsNull(), "no channel and none configured stays null")
	require.True(t, m.Description.IsNull())

	var got thresholdParamsModel
	require.False(t, m.Params.As(ctx, &got, basetypesObjectAsOptions()).HasError())
	require.Equal(t, "60s", got.Resolution.ValueString())
	require.Equal(t, "sum($g) | group by service_name", got.Query.ValueString())
	require.Equal(t, "alert", got.AbsentPoints.ValueString())
	require.True(t, got.Recovery.IsNull())

	var ri repeatIntervalModel
	require.False(t, m.RepeatInterval.As(ctx, &ri, basetypesObjectAsOptions()).HasError())
	require.Equal(t, "1h", ri.Interval.ValueString())
}

func TestApply_rejectsAnotherType(t *testing.T) {
	r := &monitorResource{kind: kindByName(t, "threshold")}
	m := testModel(t, r.kind, types.ObjectNull(r.kind.paramsAttrTypes()))
	diags := r.apply(context.Background(), map[string]any{"id": 7, "type": "error"}, m)
	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Detail(), "uptrace_error_monitor")
}

func TestFormatDuration(t *testing.T) {
	for in, want := range map[int64]string{
		0:             "0s",
		1500:          "1.5s",
		60_000:        "1m",
		90_000:        "1m30s",
		3_600_000:     "1h",
		5_400_000:     "1h30m",
		86_400_000:    "1d",
		2_592_000_000: "30d",
		90_000_000:    "25h",
	} {
		require.Equal(t, want, durationValue(in, types.StringNull()).ValueString(), "%d ms", in)
	}
}

func basetypesObjectAsOptions() basetypes.ObjectAsOptions { return basetypes.ObjectAsOptions{} }
