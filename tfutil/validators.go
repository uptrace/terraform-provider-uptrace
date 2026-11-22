package tfutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// LowercaseEmailValidator rejects non-normalized email input at plan time
// to avoid drift against server-normalized state. The backend lowercases
// and trims emails before persisting.
type LowercaseEmailValidator struct{}

func (LowercaseEmailValidator) Description(context.Context) string {
	return "email must be lowercase and trimmed"
}

func (v LowercaseEmailValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (LowercaseEmailValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if raw != normalized {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"email must be lowercase and trimmed",
			fmt.Sprintf("got %q; write %q instead. "+
				"The backend lowercases emails, so mixed-case input would cause "+
				"a perpetual diff against state.", raw, normalized),
		)
	}
}

// TrimmedNameValidator rejects a name with surrounding whitespace at plan
// time. The backend trims the name of a project, a team and an org before it
// stores it, so the applied value would differ from the planned one, and
// Terraform would report an inconsistent result after the write.
type TrimmedNameValidator struct{}

func (TrimmedNameValidator) Description(context.Context) string {
	return "name must not begin or end with whitespace"
}

func (v TrimmedNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (TrimmedNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	trimmed := strings.TrimSpace(raw)
	if raw != trimmed {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"name must not begin or end with whitespace",
			fmt.Sprintf("got %q; write %q instead. "+
				"The backend trims the name, so the stored value would differ "+
				"from the configured one.", raw, trimmed),
		)
	}
}
