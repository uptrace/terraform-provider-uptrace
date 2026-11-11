package team

import (
	"testing"

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
