package tfutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// FromProviderData unwraps the provider's ProviderData. Nil ProviderData
// (pre-Configure) returns nil silently; a wrong type emits a diagnostic.
func FromProviderData[T any](providerData any, diags *diag.Diagnostics) *T {
	if providerData == nil {
		return nil
	}
	c, ok := providerData.(*T)
	if !ok {
		diags.AddError(
			"unexpected provider data type",
			fmt.Sprintf("expected %T, got %T", new(T), providerData),
		)
		return nil
	}
	return c
}

// ImportField describes one segment of a compound import ID. Parse is
// optional; when non-nil, the segment value is passed to it so a malformed
// ID surfaces a clear, attribute-specific diagnostic at import time instead
// of failing later with a generic parse error in the first Read.
type ImportField struct {
	Name  string
	Parse func(string) error
}

// ImportStateCompoundID parses "a:b:...:id" and writes each segment to its
// attribute. The last field is mapped to "id".
func ImportStateCompoundID(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
	fields ...ImportField,
) {
	parts, ok := parseCompoundImportID(req.ID, fields, &resp.Diagnostics)
	if !ok {
		return
	}
	for i, f := range fields {
		attr := f.Name
		if i == len(fields)-1 {
			attr = "id"
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(attr), parts[i])...)
	}
}

// ImportStateJunctionID is ImportStateCompoundID for resources whose `id`
// mirrors the last named segment: each segment is written to its named
// attribute and the last one is also written to `id`.
func ImportStateJunctionID(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
	fields ...ImportField,
) {
	parts, ok := parseCompoundImportID(req.ID, fields, &resp.Diagnostics)
	if !ok {
		return
	}
	for i, f := range fields {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(f.Name), parts[i])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[len(parts)-1])...)
}

// parseCompoundImportID splits and validates a compound import ID.
func parseCompoundImportID(id string, fields []ImportField, diags *diag.Diagnostics) ([]string, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != len(fields) {
		names := make([]string, len(fields))
		for i, f := range fields {
			names[i] = f.Name
		}
		diags.AddError(
			"invalid import ID",
			fmt.Sprintf("expected format <%s>, got %q", strings.Join(names, ">:<"), id),
		)
		return nil, false
	}
	for i, f := range fields {
		if parts[i] == "" {
			diags.AddError(
				"invalid import ID",
				fmt.Sprintf("%s segment must not be empty", f.Name),
			)
			return nil, false
		}
		if f.Parse != nil {
			if err := f.Parse(parts[i]); err != nil {
				diags.AddError(
					fmt.Sprintf("invalid %s in import ID", f.Name),
					err.Error(),
				)
				return nil, false
			}
		}
	}
	return parts, true
}

// ParseInt64 validates s as a base-10 int64. Use with ImportField.Parse.
func ParseInt64(s string) error {
	_, err := strconv.ParseInt(s, 10, 64)
	return err
}

// ParseUint32 validates s as a base-10 uint32. Use with ImportField.Parse.
func ParseUint32(s string) error {
	_, err := strconv.ParseUint(s, 10, 32)
	return err
}

// ParseUint64 validates s as a base-10 uint64. Use with ImportField.Parse.
func ParseUint64(s string) error {
	_, err := strconv.ParseUint(s, 10, 64)
	return err
}
