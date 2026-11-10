package tfutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// StringFromPtr maps *string → types.String; nil or "" → Null.
func StringFromPtr(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// PreserveIfEmpty returns the API value, falling back to prior when the API
// redacted it. Use for Sensitive fields: the server answers a secret empty or
// masked with bullets ("••••••"), never as it was set.
func PreserveIfEmpty(api string, prior types.String) types.String {
	if Redacted(api) {
		return prior
	}
	return types.StringValue(api)
}

// PreserveIfEmptyPtr is PreserveIfEmpty for *string API fields.
func PreserveIfEmptyPtr(api *string, prior types.String) types.String {
	if api == nil {
		return prior
	}
	return PreserveIfEmpty(*api, prior)
}

// Redacted reports whether the server withheld a secret value: it sends an
// empty string or a mask of bullets in its place.
func Redacted(api string) bool {
	return api == "" || strings.Contains(api, "\u2022")
}

// EnumToValue maps *Enum (string-backed) → types.String; nil or "" → Null.
func EnumToValue[T ~string](p *T) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(string(*p))
}

// EnumToValueOrDefault is EnumToValue that falls back to a schema default
// instead of Null, so attributes with a Default stay consistent with plan.
func EnumToValueOrDefault[T ~string](p *T, fallback string) types.String {
	if p != nil && *p != "" {
		return types.StringValue(string(*p))
	}
	return types.StringValue(fallback)
}

// IntPtrToValue maps *int → types.Int64. Nil or zero with a null prior → Null;
// otherwise the API value surfaces so drift stays visible.
func IntPtrToValue(api *int, prior types.Int64) types.Int64 {
	if api == nil {
		return prior
	}
	if *api == 0 && prior.IsNull() {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*api))
}

// PriorString extracts a types.String field from a possibly-nil block pointer.
func PriorString[T any](prior *T, get func(*T) types.String) types.String {
	if prior == nil {
		return types.StringNull()
	}
	return get(prior)
}

// PriorInt64 is PriorString for types.Int64 fields.
func PriorInt64[T any](prior *T, get func(*T) types.Int64) types.Int64 {
	if prior == nil {
		return types.Int64Null()
	}
	return get(prior)
}

// PreferPrior returns prior when set, else fallback. Use when the backend
// normalizes a user-supplied value and state must keep the user's form.
func PreferPrior(prior types.String, fallback types.String) types.String {
	if !prior.IsNull() && !prior.IsUnknown() {
		return prior
	}
	return fallback
}

// PreserveOptional returns apiValue when userSet, else null. Use for Optional
// attributes where the backend fills a default the provider must not surface.
func PreserveOptional[T attr.Value](userSet bool, apiValue T, null T) T {
	if userSet {
		return apiValue
	}
	return null
}

// Float32PtrToFloat64 maps *float32 → types.Float64; nil → Null.
func Float32PtrToFloat64(p *float32) types.Float64 {
	if p == nil {
		return types.Float64Null()
	}
	return types.Float64Value(float64(*p))
}

// IntPtrToInt64 maps *int → types.Int64; nil → Null.
func IntPtrToInt64(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// IntSetFromSlice maps a []int API response to a types.Set of stringified IDs.
// An explicit empty prior round-trips as empty; otherwise empty API → SetNull.
func IntSetFromSlice(xs []int, prior types.Set) types.Set {
	if len(xs) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior
		}
		return types.SetNull(types.StringType)
	}
	vals := make([]attr.Value, len(xs))
	for i, x := range xs {
		vals[i] = types.StringValue(strconv.Itoa(x))
	}
	s, _ := types.SetValue(types.StringType, vals)
	return s
}

// Uint64sFromSet decodes a types.Set of string-encoded IDs into []uint64. A
// null or unknown set is an empty, non-nil slice, so a request that requires
// the list sends [] and never null. A non-decimal element emits an attribute
// diagnostic.
func Uint64sFromSet(ctx context.Context, attrPath path.Path, s types.Set) ([]uint64, diag.Diagnostics) {
	var diags diag.Diagnostics
	ids := []uint64{}
	if s.IsNull() || s.IsUnknown() {
		return ids, diags
	}
	var vals []types.String
	diags.Append(s.ElementsAs(ctx, &vals, false)...)
	if diags.HasError() {
		return nil, diags
	}
	for _, v := range vals {
		str := v.ValueString()
		id, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			diags.AddAttributeError(
				attrPath,
				"invalid ID",
				fmt.Sprintf("expected a decimal integer, got %q: %s", str, err.Error()),
			)
			return nil, diags
		}
		ids = append(ids, id)
	}
	return ids, diags
}

// SetFromUint64s maps a []uint64 API response to a types.Set of stringified
// IDs. A nil or empty slice is an empty set, not a null one.
func SetFromUint64s(ids []uint64) (types.Set, diag.Diagnostics) {
	vals := make([]attr.Value, len(ids))
	for i, id := range ids {
		vals[i] = types.StringValue(strconv.FormatUint(id, 10))
	}
	return types.SetValue(types.StringType, vals)
}

// SliceFromIntSet decodes a types.Set of string-encoded IDs into []int.
// A non-decimal element emits an attribute diagnostic.
func SliceFromIntSet(ctx context.Context, attrPath path.Path, s types.Set) ([]int, diag.Diagnostics) {
	var diags diag.Diagnostics
	if s.IsNull() || s.IsUnknown() {
		return nil, diags
	}
	var vals []types.String
	diags.Append(s.ElementsAs(ctx, &vals, false)...)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]int, 0, len(vals))
	for _, v := range vals {
		str := v.ValueString()
		n, err := strconv.Atoi(str)
		if err != nil {
			diags.AddAttributeError(
				attrPath,
				"invalid ID",
				fmt.Sprintf("expected a decimal integer, got %q: %s", str, err.Error()),
			)
			return nil, diags
		}
		out = append(out, n)
	}
	return out, diags
}
