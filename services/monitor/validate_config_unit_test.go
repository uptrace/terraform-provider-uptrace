package monitor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/testutil/tfconfig"
)

// The server folds a window rule pair on save: an at_least count at or above
// num_eval_points becomes at_all_times with no count, and a count under any
// other rule is zeroed. A plan that holds such a pair would end the apply
// inconsistent (terraform:G3), so the configuration refuses it. The server
// reads a count without a rule as at_least, so that pair follows the at_least
// cases. A configuration that omits num_eval_points has no width to compare
// here: the plan holds it (TestModifyPlan_refusesACountAtThePlannedWidth).
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
		{"at_least without a count is refused by the server", map[string]tftypes.Value{
			"window_rule": str("at_least")}, true},
		{"a count under on_average is zeroed", map[string]tftypes.Value{
			"window_rule": str("on_average"), "min_breach_points": num(3)}, true},
		{"a count without a rule reads as at_least", map[string]tftypes.Value{
			"min_breach_points": num(3)}, false},
		{"a count without a rule at the window width folds to at_all_times", map[string]tftypes.Value{
			"min_breach_points": num(3), "num_eval_points": num(3)}, true},
		{"a zero count without a rule stays", map[string]tftypes.Value{
			"min_breach_points": num(0)}, false},
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

// The server reads a positive count without a rule as at_least. The plan
// modifier of window_rule keeps the rule of the state for a configuration
// that leaves it out, and a rule that does not count would make the server
// zero the count (terraform:G3). So the plan states at_least for that pair.
// It states a count of 0 for a rule that does not count and has no count.
func TestModifyPlan_plansAtLeastForACountWithoutARule(t *testing.T) {
	ctx := context.Background()
	r := &monitorResource{kind: kindByName(t, "threshold")}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	paramsType := schemaResp.Schema.Attributes["params"].GetType().TerraformType(ctx)

	raw := func(t *testing.T, params map[string]tftypes.Value) tftypes.Value {
		t.Helper()
		return tfconfig.ConfigOf(t, schemaResp.Schema, map[string]tftypes.Value{
			"project_id": tftypes.NewValue(tftypes.String, "1"),
			"name":       tftypes.NewValue(tftypes.String, "cpu"),
			"params":     tfconfig.ObjectOf(t, paramsType, params),
		}).Raw
	}
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	num := func(n int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, n) }
	stored := map[string]tftypes.Value{"window_rule": str("at_all_times"), "min_breach_points": num(0)}

	for _, tc := range []struct {
		name   string
		config map[string]tftypes.Value
		// planned is what the attribute plan modifiers leave: the
		// configuration, with the rule of the state where it gives none.
		planned   map[string]tftypes.Value
		want      string
		wantCount int64
	}{
		// The state here holds at_least with a count, and the plan modifier
		// keeps that count. The server stores 0 under the new rule.
		{"a rule that does not count, without a count",
			map[string]tftypes.Value{"window_rule": str("on_average")},
			map[string]tftypes.Value{"window_rule": str("on_average"), "min_breach_points": num(3)},
			"on_average", 0},
		{"a count without a rule",
			map[string]tftypes.Value{"min_breach_points": num(3)},
			map[string]tftypes.Value{"window_rule": str("at_all_times"), "min_breach_points": num(3)},
			"at_least", 3},
		{"a count under a rule that the configuration states",
			map[string]tftypes.Value{"window_rule": str("at_least"), "min_breach_points": num(3)},
			map[string]tftypes.Value{"window_rule": str("at_least"), "min_breach_points": num(3)},
			"at_least", 3},
		{"no count and no rule",
			map[string]tftypes.Value{},
			stored,
			"at_all_times", 0},
		{"a zero count without a rule",
			map[string]tftypes.Value{"min_breach_points": num(0)},
			stored,
			"at_all_times", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := resource.ModifyPlanRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw(t, tc.config)},
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw(t, tc.planned)},
				State:  tfsdk.State{Schema: schemaResp.Schema, Raw: raw(t, stored)},
			}
			resp := resource.ModifyPlanResponse{Plan: req.Plan}
			r.ModifyPlan(ctx, req, &resp)
			require.False(t, resp.Diagnostics.HasError(), "diagnostics: %v", resp.Diagnostics)

			var rule types.String
			diags := resp.Plan.GetAttribute(ctx, path.Root("params").AtName("window_rule"), &rule)
			require.False(t, diags.HasError(), "diagnostics: %v", diags)
			require.Equal(t, tc.want, rule.ValueString())

			var count types.Int64
			diags = resp.Plan.GetAttribute(ctx, path.Root("params").AtName("min_breach_points"), &count)
			require.False(t, diags.HasError(), "diagnostics: %v", diags)
			require.Equal(t, tc.wantCount, count.ValueInt64())
		})
	}
}

// A configuration that omits num_eval_points plans the stored width, or the
// server default for a new monitor. An at_least count at or above that width
// would fold to at_all_times on save (terraform:G3), so the plan refuses it,
// and a count under a stored width wider than the default stays.
func TestModifyPlan_refusesACountAtThePlannedWidth(t *testing.T) {
	ctx := context.Background()
	r := &monitorResource{kind: kindByName(t, "threshold")}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	paramsType := schemaResp.Schema.Attributes["params"].GetType().TerraformType(ctx)

	raw := func(t *testing.T, params map[string]tftypes.Value) tftypes.Value {
		t.Helper()
		return tfconfig.ConfigOf(t, schemaResp.Schema, map[string]tftypes.Value{
			"project_id": tftypes.NewValue(tftypes.String, "1"),
			"name":       tftypes.NewValue(tftypes.String, "cpu"),
			"params":     tfconfig.ObjectOf(t, paramsType, params),
		}).Raw
	}
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	num := func(n int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, n) }
	unknown := tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	stored := func(width int64) map[string]tftypes.Value {
		return map[string]tftypes.Value{"window_rule": str("at_all_times"), "num_eval_points": num(width)}
	}

	for _, tc := range []struct {
		name    string
		config  map[string]tftypes.Value
		width   tftypes.Value // the planned num_eval_points
		state   map[string]tftypes.Value
		refused bool
	}{
		{"a new monitor at the default width",
			map[string]tftypes.Value{"window_rule": str("at_least"), "min_breach_points": num(5)}, unknown, nil, true},
		{"a new monitor under the default width",
			map[string]tftypes.Value{"window_rule": str("at_least"), "min_breach_points": num(4)}, unknown, nil, false},
		{"a count without a rule at the stored width",
			map[string]tftypes.Value{"min_breach_points": num(3)}, num(3), stored(3), true},
		{"a count without a rule under a stored width wider than the default",
			map[string]tftypes.Value{"min_breach_points": num(6)}, num(10), stored(10), false},
		{"a configured width is the check of ValidateConfig",
			map[string]tftypes.Value{"min_breach_points": num(3), "num_eval_points": num(3)}, num(3), stored(3), false},
		{"a rule that does not count",
			map[string]tftypes.Value{"window_rule": str("on_average")}, num(3), stored(3), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planned := map[string]tftypes.Value{"num_eval_points": tc.width}
			for k, v := range tc.config {
				planned[k] = v
			}
			req := resource.ModifyPlanRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw(t, tc.config)},
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw(t, planned)},
				State:  tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)},
			}
			if tc.state != nil {
				req.State.Raw = raw(t, tc.state)
			}
			resp := resource.ModifyPlanResponse{Plan: req.Plan}
			r.ModifyPlan(ctx, req, &resp)
			require.Equal(t, tc.refused, resp.Diagnostics.HasError(), "diagnostics: %v", resp.Diagnostics)
		})
	}
}
