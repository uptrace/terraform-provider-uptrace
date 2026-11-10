package tfutil

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestLowercaseEmailValidator_rejectsMixedCase(t *testing.T) {
	var resp validator.StringResponse
	LowercaseEmailValidator{}.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("email"),
		ConfigValue: types.StringValue("Foo@Bar.com"),
	}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "email must be lowercase and trimmed")
}

func TestLowercaseEmailValidator_acceptsNormalized(t *testing.T) {
	var resp validator.StringResponse
	LowercaseEmailValidator{}.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("email"),
		ConfigValue: types.StringValue("foo@bar.com"),
	}, &resp)
	require.False(t, resp.Diagnostics.HasError())
}

func TestLowercaseEmailValidator_skipsNullUnknown(t *testing.T) {
	var resp validator.StringResponse
	LowercaseEmailValidator{}.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("email"),
		ConfigValue: types.StringNull(),
	}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	resp = validator.StringResponse{}
	LowercaseEmailValidator{}.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("email"),
		ConfigValue: types.StringUnknown(),
	}, &resp)
	require.False(t, resp.Diagnostics.HasError())
}
