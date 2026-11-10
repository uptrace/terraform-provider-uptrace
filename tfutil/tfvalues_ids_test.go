package tfutil

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestUint64sFromSet_nullAndUnknownAreEmpty(t *testing.T) {
	for _, s := range []types.Set{types.SetNull(types.StringType), types.SetUnknown(types.StringType)} {
		ids, diags := Uint64sFromSet(context.Background(), path.Root("team_ids"), s)
		require.False(t, diags.HasError())
		require.NotNil(t, ids, "a request that requires the list sends [], not null")
		require.Empty(t, ids)
	}
}

func TestUint64sFromSet_roundTrips(t *testing.T) {
	set, diags := SetFromUint64s([]uint64{3, 18446744073709551615})
	require.False(t, diags.HasError())

	ids, diags := Uint64sFromSet(context.Background(), path.Root("team_ids"), set)
	require.False(t, diags.HasError())
	require.ElementsMatch(t, []uint64{3, 18446744073709551615}, ids)
}

func TestUint64sFromSet_rejectsANonDecimalElement(t *testing.T) {
	set := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("x")})

	_, diags := Uint64sFromSet(context.Background(), path.Root("team_ids"), set)
	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Detail(), `got "x"`)
}

func TestSetFromUint64s_nilIsAnEmptySet(t *testing.T) {
	set, diags := SetFromUint64s(nil)
	require.False(t, diags.HasError())
	require.False(t, set.IsNull())
	require.Empty(t, set.Elements())
}
