package user

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

func TestUserResource_Schema_emailRequiresReplace(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&UserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	emailAttr, ok := schemaResp.Schema.Attributes["email"].(rschema.StringAttribute)
	require.True(t, ok, "email must be a StringAttribute")
	require.True(t, emailAttr.Required, "email must be required")
	require.NotEmpty(t, emailAttr.PlanModifiers, "email must have a RequiresReplace plan modifier")
}

func TestUserResource_Schema_emailRejectsMixedCase(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&UserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	emailAttr, ok := schemaResp.Schema.Attributes["email"].(rschema.StringAttribute)
	require.True(t, ok)

	var resp validator.StringResponse
	for _, v := range emailAttr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("email"),
			ConfigValue: types.StringValue("ALICE@Org.com"),
		}, &resp)
	}
	require.True(t, resp.Diagnostics.HasError())
}
