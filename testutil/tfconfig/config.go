package tfconfig

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"
)

// ConfigOf builds the configuration of a resource for a unit test of its
// ValidateConfig: the attributes in set hold the given values, and every
// other attribute of s is null, as an attribute the configuration leaves out
// is.
func ConfigOf(t *testing.T, s schema.Schema, set map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	return tfsdk.Config{Schema: s, Raw: ObjectOf(t, s.Type().TerraformType(context.Background()), set)}
}

// ObjectOf builds a value of the object type typ, with the attributes in set
// holding the given values and every other attribute null. A nested object
// attribute takes the value ObjectOf builds for its own type.
func ObjectOf(t *testing.T, typ tftypes.Type, set map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	obj, ok := typ.(tftypes.Object)
	require.True(t, ok, "the type must be an object, got %s", typ)
	values := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, attrType := range obj.AttributeTypes {
		value, ok := set[name]
		if !ok {
			value = tftypes.NewValue(attrType, nil)
		}
		values[name] = value
	}
	for name := range set {
		_, ok := obj.AttributeTypes[name]
		require.True(t, ok, "the object has no attribute %q", name)
	}
	return tftypes.NewValue(obj, values)
}
