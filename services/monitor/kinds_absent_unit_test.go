package monitor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

// The provider sends absentResolveAfter only under absent_points alert, the one
// rule that reads it. Under another rule the plan holds 0s, which the server
// stores anyway, and a server older than the key refuses a request that names
// it (terraform:QS-2).
func TestMetricParamsToAPI_sendsTheAbsentExpiryOnlyUnderAlert(t *testing.T) {
	for _, tc := range []struct {
		rule, expiry string
		sent         bool
	}{
		{"alert", "6h", true},
		{"ignore", "0s", false},
		{"zero", "0s", false},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			m := &metricParamsModel{
				Query:              types.StringValue("sum($g{})"),
				AbsentPoints:       types.StringValue(tc.rule),
				AbsentResolveAfter: types.StringValue(tc.expiry),
			}
			var diags diag.Diagnostics
			out := m.toAPI(context.Background(), &diags)
			require.False(t, diags.HasError(), "%v", diags)

			raw, err := json.Marshal(out)
			require.NoError(t, err)
			var keys map[string]any
			require.NoError(t, json.Unmarshal(raw, &keys))
			_, ok := keys["absentResolveAfter"]
			require.Equal(t, tc.sent, ok, "request params: %s", raw)
		})
	}
}
