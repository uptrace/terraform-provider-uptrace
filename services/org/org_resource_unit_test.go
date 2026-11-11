package org

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/api"
)

func TestOrgToModel_basic(t *testing.T) {
	org := &api.Org{ID: 42, Name: "Acme", Budget: 100.0}
	var m orgModel

	orgToModel(org, &m)

	require.Equal(t, types.StringValue("42"), m.ID)
	require.Equal(t, types.StringValue("Acme"), m.Name)
	require.Equal(t, types.Float64Value(100.0), m.Budget)
}

func TestOrgToModel_budgetFollowsAPI(t *testing.T) {
	model := orgModel{
		ID:     types.StringValue("41"),
		Name:   types.StringValue("old-name"),
		Budget: types.Float64Value(250),
	}

	orgToModel(&api.Org{ID: 42, Name: "new-name", Budget: 0}, &model)

	require.Equal(t, types.StringValue("42"), model.ID)
	require.Equal(t, types.StringValue("new-name"), model.Name)
	require.Equal(t, types.Float64Value(0), model.Budget, "the API always states a budget")
}
