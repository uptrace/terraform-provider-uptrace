package notifchan

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestPushoverChannelResource_SchemaPriorityRejectsOutsideRange(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&PushoverChannelResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	priorityAttr, ok := schemaResp.Schema.Attributes["priority"].(rschema.Int64Attribute)
	require.True(t, ok, "priority must be an Int64Attribute")
	require.NotEmpty(t, priorityAttr.Validators, "priority must validate the documented range")

	var badResp validator.Int64Response
	for _, v := range priorityAttr.Validators {
		v.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("priority"),
			ConfigValue: types.Int64Value(3),
		}, &badResp)
	}
	require.True(t, badResp.Diagnostics.HasError())

	var goodResp validator.Int64Response
	for _, v := range priorityAttr.Validators {
		v.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("priority"),
			ConfigValue: types.Int64Value(-2),
		}, &goodResp)
	}
	require.False(t, goodResp.Diagnostics.HasError())
}
