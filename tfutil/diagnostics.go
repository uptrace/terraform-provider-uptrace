package tfutil

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/uptrace/terraform/client"
)

// AddAPIError emits a generic API-error diagnostic. The backend's own message
// is surfaced as-is; callers don't need to special-case license-required or
// other server-owned error categories.
func AddAPIError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, client.APIErrorMessage(err))
}
