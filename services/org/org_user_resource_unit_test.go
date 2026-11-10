package org

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/stretchr/testify/require"
)

func TestOrgUserResource_Schema_orgIDRequiresReplace(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&OrgUserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	orgIDAttr, ok := schemaResp.Schema.Attributes["org_id"].(rschema.StringAttribute)
	require.True(t, ok)
	require.True(t, orgIDAttr.Required)
	require.NotEmpty(t, orgIDAttr.PlanModifiers, "org_id must be RequiresReplace")
}

func TestOrgUserResource_Schema_userIDRequiresReplace(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&OrgUserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	userIDAttr, ok := schemaResp.Schema.Attributes["user_id"].(rschema.StringAttribute)
	require.True(t, ok)
	require.True(t, userIDAttr.Required)
	require.NotEmpty(t, userIDAttr.PlanModifiers, "user_id must be RequiresReplace")
}

func TestOrgUserResource_Schema_roleIsRequired(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&OrgUserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	roleAttr, ok := schemaResp.Schema.Attributes["role"].(rschema.StringAttribute)
	require.True(t, ok)
	require.True(t, roleAttr.Required)
}
