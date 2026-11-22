package org

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

// TestOrgUserResource_ModifyPlan pins that a plan needing a create fails at
// plan time, before an apply removes anything: the server has no operation that
// creates a membership by user ID. A role change and a destroy of a membership
// already in the state pass.
func TestOrgUserResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&OrgUserResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	s := schemaResp.Schema
	objectType := s.Type().TerraformType(ctx)

	membership := func(orgID, userID any, role string) tftypes.Value {
		return tftypes.NewValue(objectType, map[string]tftypes.Value{
			"id":      tftypes.NewValue(tftypes.String, "7"),
			"org_id":  tftypes.NewValue(tftypes.String, orgID),
			"user_id": tftypes.NewValue(tftypes.String, userID),
			"role":    tftypes.NewValue(tftypes.String, role),
		})
	}
	null := tftypes.NewValue(objectType, nil)

	tests := []struct {
		name   string
		state  tftypes.Value
		plan   tftypes.Value
		refuse bool
	}{
		{"a new instance", null, membership("1", "2", "member"), true},
		{"a role change", membership("1", "2", "member"), membership("1", "2", "admin"), false},
		{"an unchanged membership", membership("1", "2", "admin"), membership("1", "2", "admin"), false},
		{"a destroy", membership("1", "2", "admin"), null, false},
		{"an org_id change, which replaces", membership("1", "2", "admin"), membership("3", "2", "admin"), true},
		{"a user_id change, which replaces", membership("1", "2", "admin"), membership("1", "4", "admin"), true},
		{
			"a user_id known only at apply",
			membership("1", "2", "admin"), membership("1", tftypes.UnknownValue, "admin"), true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := resource.ModifyPlanRequest{
				State: tfsdk.State{Schema: s, Raw: tt.state},
				Plan:  tfsdk.Plan{Schema: s, Raw: tt.plan},
			}
			resp := resource.ModifyPlanResponse{Plan: req.Plan}
			(&OrgUserResource{}).ModifyPlan(ctx, req, &resp)

			if !tt.refuse {
				require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				return
			}
			require.True(t, resp.Diagnostics.HasError())
			require.Equal(t, orgUserCreateRefused, resp.Diagnostics[0].Summary())
			require.Contains(t, resp.Diagnostics[0].Detail(), "uptrace_org_member")
		})
	}
}
