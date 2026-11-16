package team

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/api"
)

func TestTeamToModel_basic(t *testing.T) {
	pl := api.PermLevel("admin")
	team := &api.Team{ID: 7, OrgID: 42, Name: "ops", PermLevel: pl}
	var m teamModel

	teamToModel(team, &m)

	require.Equal(t, types.StringValue("7"), m.ID)
	require.Equal(t, types.StringValue("42"), m.OrgID)
	require.Equal(t, types.StringValue("ops"), m.Name)
	require.Equal(t, types.StringValue("admin"), m.PermLevel)
}

func TestTeamToModel_nilPermLevelStaysNull(t *testing.T) {
	m := teamModel{
		PermLevel: types.StringValue("edit"),
	}

	teamToModel(&api.Team{ID: 1, OrgID: 2, Name: "x"}, &m)

	require.True(t, m.PermLevel.IsNull(), "nil API permLevel must clear state")
}

func TestTeamToModel_emptyPermLevelStaysNull(t *testing.T) {
	empty := api.PermLevel("")
	m := teamModel{}

	teamToModel(&api.Team{ID: 1, OrgID: 2, Name: "x", PermLevel: empty}, &m)

	require.True(t, m.PermLevel.IsNull(), "empty-string API permLevel must surface as null")
}

func TestTeamToModel_overwritesExisting(t *testing.T) {
	pl := api.PermLevel("view")
	m := teamModel{
		ID:        types.StringValue("99"),
		OrgID:     types.StringValue("88"),
		Name:      types.StringValue("old"),
		PermLevel: types.StringValue("admin"),
	}

	teamToModel(&api.Team{ID: 1, OrgID: 2, Name: "new", PermLevel: pl}, &m)

	require.Equal(t, types.StringValue("1"), m.ID)
	require.Equal(t, types.StringValue("2"), m.OrgID)
	require.Equal(t, types.StringValue("new"), m.Name)
	require.Equal(t, types.StringValue("view"), m.PermLevel)
}

// validatePermLevel runs the validators of the perm_level schema on value.
func validatePermLevel(t *testing.T, value string) validator.StringResponse {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&TeamResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	attr, ok := schemaResp.Schema.Attributes["perm_level"].(rschema.StringAttribute)
	require.True(t, ok, "perm_level must be a StringAttribute")

	var resp validator.StringResponse
	for _, v := range attr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("perm_level"),
			ConfigValue: types.StringValue(value),
		}, &resp)
	}
	return resp
}

func TestTeamResource_Schema_permLevelNoneNamesTheUpgradePath(t *testing.T) {
	resp := validatePermLevel(t, "none")
	require.Equal(t, 1, resp.Diagnostics.ErrorsCount())
	diag := resp.Diagnostics.Errors()[0]
	require.Equal(t, `a team no longer holds "none"`, diag.Summary())
	require.Contains(t, diag.Detail(), "remove the uptrace_team_project resources of this team")
	require.Contains(t, diag.Detail(), `project_overrides = { <project_id> = "none" } on the uptrace_org_member`)
}

func TestTeamResource_Schema_permLevelOtherValues(t *testing.T) {
	resp := validatePermLevel(t, "owner")
	require.Equal(t, 1, resp.Diagnostics.ErrorsCount(), "an unknown level gets the generic refusal once")
	require.NotEqual(t, `a team no longer holds "none"`, resp.Diagnostics.Errors()[0].Summary())

	for _, level := range teamPermLevels {
		require.False(t, validatePermLevel(t, level).Diagnostics.HasError(), level)
	}
}
