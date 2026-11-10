package org

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"
)

func orgMemberSchema(t *testing.T) rschema.Schema {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&OrgMemberResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	return schemaResp.Schema
}

func TestOrgMemberResource_Schema_keyRequiresReplace(t *testing.T) {
	s := orgMemberSchema(t)
	for _, name := range []string{"org_id", "email"} {
		attr, ok := s.Attributes[name].(rschema.StringAttribute)
		require.True(t, ok, name)
		require.True(t, attr.Required, name)
		require.NotEmpty(t, attr.PlanModifiers, "%s must be RequiresReplace", name)
	}
}

func TestOrgMemberResource_Schema_serverValuesAreComputed(t *testing.T) {
	s := orgMemberSchema(t)

	role, ok := s.Attributes["role"].(rschema.StringAttribute)
	require.True(t, ok)
	require.True(t, role.Required)

	teams, ok := s.Attributes["team_ids"].(rschema.SetAttribute)
	require.True(t, ok)
	require.True(t, teams.Optional)
	require.NotNil(t, teams.Default, "an omitted team_ids is the empty set")

	for _, name := range []string{"status", "org_user_id"} {
		attr, ok := s.Attributes[name].(rschema.StringAttribute)
		require.True(t, ok, name)
		require.True(t, attr.Computed, name)
		require.False(t, attr.Optional, name)
	}
}

func TestOrgMemberResource_ImportState(t *testing.T) {
	importState := func(t *testing.T, id string) resource.ImportStateResponse {
		t.Helper()
		s := orgMemberSchema(t)
		resp := resource.ImportStateResponse{
			State: tfsdk.State{
				Schema: s,
				Raw:    tftypes.NewValue(s.Type().TerraformType(context.Background()), nil),
			},
		}
		(&OrgMemberResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		return resp
	}

	// The email is the rest of the ID after the first ":", so an address that
	// holds a ":" in a quoted local part parses too.
	for _, tc := range []struct{ id, email string }{
		{"42:dev@example.com", "dev@example.com"},
		{`42:"a:b"@example.com`, `"a:b"@example.com`},
	} {
		resp := importState(t, tc.id)
		require.False(t, resp.Diagnostics.HasError(), "%s: %v", tc.id, resp.Diagnostics)

		var orgID, email, id string
		require.False(t, resp.State.GetAttribute(context.Background(), path.Root("org_id"), &orgID).HasError())
		require.False(t, resp.State.GetAttribute(context.Background(), path.Root("email"), &email).HasError())
		require.False(t, resp.State.GetAttribute(context.Background(), path.Root("id"), &id).HasError())
		require.Equal(t, "42", orgID)
		require.Equal(t, tc.email, email)
		require.Equal(t, tc.id, id)
	}

	for _, bad := range []string{"dev@example.com", "x:dev@example.com", "42:", ":dev@example.com"} {
		resp := importState(t, bad)
		require.True(t, resp.Diagnostics.HasError(), "%q must be refused", bad)
	}
}
