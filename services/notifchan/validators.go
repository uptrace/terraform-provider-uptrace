package notifchan

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// jsonObjectValidator ensures a string attribute parses as a JSON object
// (not a scalar, array, or malformed JSON).
type jsonObjectValidator struct{}

func (jsonObjectValidator) Description(context.Context) string {
	return "must be a JSON object (e.g. constructed with jsonencode())"
}

func (jsonObjectValidator) MarkdownDescription(context.Context) string {
	return "must be a JSON object (e.g. constructed with `jsonencode()`)"
}

func (jsonObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var v any
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &v); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid JSON", err.Error())
		return
	}
	if _, ok := v.(map[string]any); !ok {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"payload must be a JSON object",
			fmt.Sprintf("got %T; use a JSON object at the top level (e.g. jsonencode({ key = \"value\" }))", v),
		)
	}
}

type nullable interface {
	IsNull() bool
	IsUnknown() bool
}

func fieldIsSet(v nullable) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// forbidField emits a diagnostic when an attribute that must not be set
// under the current configuration happens to be set.
func forbidField(v nullable, p path.Path, reason string, diags *diag.Diagnostics) {
	if fieldIsSet(v) {
		diags.AddAttributeError(
			p,
			"unexpected attribute",
			fmt.Sprintf("%s must not be set when %s.", p.String(), reason),
		)
	}
}

// requireField emits a diagnostic when a required attribute is null.
func requireField(v types.String, p path.Path, reason string, diags *diag.Diagnostics) {
	if v.IsNull() && !v.IsUnknown() {
		diags.AddAttributeError(
			p,
			p.String()+" is required",
			p.String()+" must be set when "+reason+".",
		)
	}
}
