package monitor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/testutil/tfconfig"
)

// The server folds a window rule pair on save: an at_least count at or above
// num_eval_points becomes at_all_times with no count, and a count under any
// other rule is zeroed. A plan that holds such a pair would end the apply
// inconsistent (terraform:G3), so the configuration refuses it.
func TestValidateConfig_refusesAWindowRulePairTheServerFolds(t *testing.T) {
	ctx := context.Background()
	r := &monitorResource{kind: kindByName(t, "threshold")}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	paramsType := schemaResp.Schema.Attributes["params"].GetType().TerraformType(ctx)

	validate := func(t *testing.T, params map[string]tftypes.Value) resource.ValidateConfigResponse {
		t.Helper()
		config := tfconfig.ConfigOf(t, schemaResp.Schema, map[string]tftypes.Value{
			"project_id": tftypes.NewValue(tftypes.String, "1"),
			"name":       tftypes.NewValue(tftypes.String, "cpu"),
			"params":     tfconfig.ObjectOf(t, paramsType, params),
		})
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, &resp)
		return resp
	}
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	num := func(n int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, n) }

	for _, tc := range []struct {
		name    string
		params  map[string]tftypes.Value
		refused bool
	}{
		{"at_least under the window width", map[string]tftypes.Value{
			"window_rule": str("at_least"), "min_breach_points": num(2), "num_eval_points": num(5)}, false},
		{"at_least with the default window width", map[string]tftypes.Value{
			"window_rule": str("at_least"), "min_breach_points": num(4)}, false},
		{"at_least at the window width folds to at_all_times", map[string]tftypes.Value{
			"window_rule": str("at_least"), "min_breach_points": num(5), "num_eval_points": num(5)}, true},
		{"at_least at the default window width folds to at_all_times", map[string]tftypes.Value{
			"window_rule": str("at_least"), "min_breach_points": num(5)}, true},
		{"at_least without a count is refused by the server", map[string]tftypes.Value{
			"window_rule": str("at_least")}, true},
		{"a count under on_average is zeroed", map[string]tftypes.Value{
			"window_rule": str("on_average"), "min_breach_points": num(3)}, true},
		{"a count under the default rule is zeroed", map[string]tftypes.Value{
			"min_breach_points": num(3)}, true},
		{"a zero count under another rule stays", map[string]tftypes.Value{
			"window_rule": str("in_total"), "min_breach_points": num(0)}, false},
		{"no pair", map[string]tftypes.Value{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := validate(t, tc.params)
			require.Equal(t, tc.refused, resp.Diagnostics.HasError(), "diagnostics: %v", resp.Diagnostics)
		})
	}
}
