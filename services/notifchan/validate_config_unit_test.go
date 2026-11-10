package notifchan

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/testutil/tfconfig"
)

// The server accepts match_all = false with no linked monitor: an empty list
// detaches every monitor, and an omitted one leaves the links to the monitors
// that name the channel in their channel_ids, as the attribute description and
// the README state. The configuration must not refuse either.
func TestValidateConfig_restrictedChannelNeedsNoMonitorIDs(t *testing.T) {
	ctx := context.Background()
	// The Slack channel keeps a ValidateConfig for its auth method, and runs
	// no other check when the method is unset.
	r := &SlackChannelResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	validate := func(t *testing.T, monitorIDs tftypes.Value) resource.ValidateConfigResponse {
		t.Helper()
		config := tfconfig.ConfigOf(t, schemaResp.Schema, map[string]tftypes.Value{
			"project_id":  tftypes.NewValue(tftypes.String, "1"),
			"name":        tftypes.NewValue(tftypes.String, "ops"),
			"webhook_url": tftypes.NewValue(tftypes.String, "https://hooks.slack.example/hook"),
			"match_all":   tftypes.NewValue(tftypes.Bool, false),
			"priorities": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
				tftypes.NewValue(tftypes.String, "high"),
			}),
			"monitor_ids": monitorIDs,
		})
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, &resp)
		return resp
	}

	t.Run("an empty list detaches every monitor", func(t *testing.T) {
		resp := validate(t, tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{}))
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)
	})

	t.Run("an omitted list leaves the links to the monitors", func(t *testing.T) {
		resp := validate(t, tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil))
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)
	})
}
