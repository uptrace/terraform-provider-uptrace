package tfutil

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/require"
)

func TestImportStateCompoundID_rejectsInvalidNumericSegments(t *testing.T) {
	req := resource.ImportStateRequest{ID: "abc:123"}
	resp := &resource.ImportStateResponse{}

	ImportStateCompoundID(context.Background(), req, resp,
		ImportField{Name: "org_id", Parse: ParseUint64},
		ImportField{Name: "project_id", Parse: ParseUint32},
	)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid org_id in import ID")
}

func TestImportStateCompoundID_rejectsInvalidFinalSegment(t *testing.T) {
	req := resource.ImportStateRequest{ID: "7:not-a-token"}
	resp := &resource.ImportStateResponse{}

	ImportStateCompoundID(context.Background(), req, resp,
		ImportField{Name: "project_id", Parse: ParseUint32},
		ImportField{Name: "token_id", Parse: ParseUint64},
	)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid token_id in import ID")
}
