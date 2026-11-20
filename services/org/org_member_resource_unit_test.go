package org

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/api"
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

		// The import reads the overrides the server holds (ADR-0466).
		var overrides types.Map
		require.False(t, resp.State.GetAttribute(context.Background(), path.Root("project_overrides"), &overrides).HasError())
		require.False(t, overrides.IsNull(), "an import manages the overrides")
		require.Empty(t, overrides.Elements())
	}

	for _, bad := range []string{"dev@example.com", "x:dev@example.com", "42:", ":dev@example.com"} {
		resp := importState(t, bad)
		require.True(t, resp.Diagnostics.HasError(), "%q must be refused", bad)
	}
}

func TestOrgMemberResource_Schema_projectOverridesAreOptional(t *testing.T) {
	overrides, ok := orgMemberSchema(t).Attributes["project_overrides"].(rschema.MapAttribute)
	require.True(t, ok)
	require.True(t, overrides.Optional)
	// Neither computed nor defaulted: an omitted map stays null, and the
	// server keeps the overrides it holds (ADR-0466).
	require.False(t, overrides.Computed)
}

// validateOrgMember runs ValidateConfig on a configuration with role and
// overrides, a nil map being an omitted attribute.
func validateOrgMember(t *testing.T, role string, overrides map[string]string) resource.ValidateConfigResponse {
	t.Helper()
	s := orgMemberSchema(t)
	ctx := context.Background()

	overridesValue := tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil)
	if overrides != nil {
		levels := make(map[string]tftypes.Value, len(overrides))
		for key, level := range overrides {
			levels[key] = tftypes.NewValue(tftypes.String, level)
		}
		overridesValue = tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, levels)
	}
	objectType := s.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, typ := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(typ, nil)
	}
	values["org_id"] = tftypes.NewValue(tftypes.String, "1")
	values["email"] = tftypes.NewValue(tftypes.String, "dev@example.com")
	values["role"] = tftypes.NewValue(tftypes.String, role)
	values["project_overrides"] = overridesValue

	resp := resource.ValidateConfigResponse{}
	(&OrgMemberResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objectType, values)},
	}, &resp)
	return resp
}

// TestOrgMemberResource_ValidateConfig pins the plan-time refusal of an
// override that the server refuses: a level below the one the role grants on
// every project, other than none (app core/DESIGN.md D7), a key that is not
// a project ID, and a project ID with a leading zero, which the state would
// write back without it.
func TestOrgMemberResource_ValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		role      string
		overrides map[string]string
		refused   bool
	}{
		{role: "admin", overrides: nil},
		{role: "admin", overrides: map[string]string{}},
		{role: "admin", overrides: map[string]string{"7": "none"}},
		{role: "admin", overrides: map[string]string{"7": "admin"}},
		{role: "admin", overrides: map[string]string{"7": "edit"}, refused: true},
		{role: "owner", overrides: map[string]string{"7": "view"}, refused: true},
		{role: "viewer", overrides: map[string]string{"7": "view"}},
		{role: "viewer", overrides: map[string]string{"7": "edit"}},
		{role: "member", overrides: map[string]string{"7": "view"}},
		{role: "collaborator", overrides: map[string]string{"7": "none"}},
		{role: "billing_manager", overrides: map[string]string{"7": "admin"}},
		{role: "member", overrides: map[string]string{"prod": "none"}, refused: true},
		{role: "member", overrides: map[string]string{"4294967296": "none"}, refused: true},
		{role: "member", overrides: map[string]string{"007": "none"}, refused: true},
		{role: "member", overrides: map[string]string{"07": "view", "7": "view"}, refused: true},
	} {
		resp := validateOrgMember(t, tc.role, tc.overrides)
		require.Equalf(t, tc.refused, resp.Diagnostics.HasError(),
			"role %s, overrides %v: %v", tc.role, tc.overrides, resp.Diagnostics)
	}
}

func TestProjectOverridesFromMap(t *testing.T) {
	got, diags := projectOverridesFromMap(types.MapNull(types.StringType))
	require.False(t, diags.HasError())
	require.Nil(t, got, "a null map sends no key, so the server keeps its overrides")

	got, diags = projectOverridesFromMap(types.MapValueMust(types.StringType, map[string]attr.Value{}))
	require.False(t, diags.HasError())
	require.NotNil(t, got, "an empty map sends an empty list, which removes every override")
	require.Empty(t, *got)

	got, diags = projectOverridesFromMap(types.MapValueMust(types.StringType, map[string]attr.Value{
		"12": types.StringValue("none"),
		"3":  types.StringValue("edit"),
	}))
	require.False(t, diags.HasError())
	require.Equal(t, []api.OrgMemberProjectOverride{
		{ProjectID: 3, PermLevel: api.PermLevelEdit},
		{ProjectID: 12, PermLevel: api.PermLevelNone},
	}, *got, "sorted by project ID")
}

func TestApplyOrgMember_projectOverrides(t *testing.T) {
	member := &api.OrgMember{
		Status: api.OrgMemberStatusPending, Email: "dev@example.com", Role: api.UserRoleMember,
		TeamIDs:          []uint64{},
		ProjectOverrides: []api.OrgMemberProjectOverride{{ProjectID: 3, PermLevel: api.PermLevelNone}},
	}

	unmanaged := orgMemberModel{OrgID: types.StringValue("1"), ProjectOverrides: types.MapNull(types.StringType)}
	require.False(t, applyOrgMember(&unmanaged, member).HasError())
	require.True(t, unmanaged.ProjectOverrides.IsNull(), "an unmanaged map stays null")

	managed := orgMemberModel{
		OrgID:            types.StringValue("1"),
		ProjectOverrides: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}
	require.False(t, applyOrgMember(&managed, member).HasError())
	require.Equal(t, types.MapValueMust(types.StringType, map[string]attr.Value{
		"3": types.StringValue("none"),
	}), managed.ProjectOverrides)
}

// TestOrgMemberResource_ValidateConfig_detail pins the words of the plan-time
// refusal, which are the server's: a predicate of the override, with the level
// it got (app pkg/validerr/README.md).
func TestOrgMemberResource_ValidateConfig_detail(t *testing.T) {
	resp := validateOrgMember(t, "admin", map[string]string{"7": "edit"})
	require.Len(t, resp.Diagnostics, 1)
	require.Equal(t, "project override below the role", resp.Diagnostics[0].Summary())
	require.Equal(t,
		"must be admin or none, because the admin role grants admin on every project (got edit)",
		resp.Diagnostics[0].Detail())
}
