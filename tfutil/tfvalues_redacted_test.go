package tfutil

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestPreserveIfEmpty_maskedValueKeepsPrior(t *testing.T) {
	prior := types.StringValue("xoxb-123456789")
	require.Equal(t, prior, PreserveIfEmpty("••••••", prior))
	require.Equal(t, prior, PreserveIfEmpty("https://hooks.slack.com/••••", prior))
	require.Equal(t, prior, PreserveIfEmpty("", prior))
	require.Equal(t, types.StringValue("#alerts"), PreserveIfEmpty("#alerts", prior))
	require.True(t, PreserveIfEmpty("••••••", types.StringNull()).IsNull(), "an import keeps no mask in state")
}
