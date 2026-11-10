package notifchan

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestRunAlertmanagerAuthValidators(t *testing.T) {
	tests := []struct {
		name  string
		model alertmanagerChannelModel
		ok    bool
	}{
		{
			"no auth and no credentials ok",
			alertmanagerChannelModel{
				AuthMethod: types.StringNull(),
				Username:   types.StringNull(),
				Password:   types.StringNull(),
				Token:      types.StringNull(),
			},
			true,
		},
		{
			"credentials require auth_method",
			alertmanagerChannelModel{
				AuthMethod: types.StringNull(),
				Username:   types.StringValue("user"),
				Password:   types.StringNull(),
				Token:      types.StringNull(),
			},
			false,
		},
		{
			"basic_auth requires username and password",
			alertmanagerChannelModel{
				AuthMethod: types.StringValue("basic_auth"),
				Username:   types.StringValue("user"),
				Password:   types.StringNull(),
				Token:      types.StringNull(),
			},
			false,
		},
		{
			"basic_auth ok",
			alertmanagerChannelModel{
				AuthMethod: types.StringValue("basic_auth"),
				Username:   types.StringValue("user"),
				Password:   types.StringValue("pass"),
				Token:      types.StringNull(),
			},
			true,
		},
		{
			"bearer forbids basic credentials",
			alertmanagerChannelModel{
				AuthMethod: types.StringValue("bearer"),
				Username:   types.StringValue("user"),
				Password:   types.StringNull(),
				Token:      types.StringValue("token"),
			},
			false,
		},
		{
			"none forbids credentials",
			alertmanagerChannelModel{
				AuthMethod: types.StringValue("none"),
				Username:   types.StringNull(),
				Password:   types.StringNull(),
				Token:      types.StringValue("token"),
			},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			runAlertmanagerAuthValidators(&tc.model, &diags)
			require.Equal(t, !tc.ok, diags.HasError())
		})
	}
}
