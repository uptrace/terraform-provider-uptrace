package org

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/client"
)

// readOrgMemberData runs the data source's Read for org 1 and email against a
// server that answers status and body for the membership read.
func readOrgMemberData(
	t *testing.T, email string, status int, body string,
) (orgMemberDataModel, datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/v1/orgs/1/members/by-email", r.URL.Path)
		require.Equal(t, email, r.URL.Query().Get("email"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(srv.URL, "token", "test")
	require.NoError(t, err)

	var schemaResp datasource.SchemaResponse
	(&OrgMemberDataSource{}).Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	s := schemaResp.Schema

	objectType := s.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, typ := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(typ, nil)
	}
	values["org_id"] = tftypes.NewValue(tftypes.String, "1")
	values["email"] = tftypes.NewValue(tftypes.String, email)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objectType, values)}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(objectType, nil)}}
	(&OrgMemberDataSource{client: c}).Read(ctx, req, &resp)

	var got orgMemberDataModel
	if !resp.Diagnostics.HasError() {
		require.False(t, resp.State.Get(ctx, &got).HasError())
	}
	return got, resp
}

func TestOrgMemberDataSource_Read(t *testing.T) {
	t.Run("an active member carries its org user ID", func(t *testing.T) {
		got, resp := readOrgMemberData(t, "owner@example.com", http.StatusOK, `{"member": {
			"status": "active", "email": "owner@example.com", "role": "owner",
			"teamIds": [5, 3], "projectOverrides": [], "orgUserId": 42}}`)
		require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

		require.Equal(t, "1:owner@example.com", got.ID.ValueString())
		require.Equal(t, "active", got.Status.ValueString())
		require.Equal(t, "owner", got.Role.ValueString())
		require.Equal(t, "42", got.OrgUserID.ValueString())
		var teamIDs []string
		require.False(t, got.TeamIDs.ElementsAs(context.Background(), &teamIDs, false).HasError())
		require.ElementsMatch(t, []string{"3", "5"}, teamIDs)
	})

	t.Run("a pending membership has no org user ID", func(t *testing.T) {
		got, resp := readOrgMemberData(t, "invitee@example.com", http.StatusOK, `{"member": {
			"status": "pending", "email": "invitee@example.com", "role": "member",
			"teamIds": [], "projectOverrides": [], "orgUserId": null}}`)
		require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

		require.Equal(t, "pending", got.Status.ValueString())
		require.True(t, got.OrgUserID.IsNull())
		require.Zero(t, len(got.TeamIDs.Elements()))
	})

	t.Run("a missing membership fails and names the SSO case", func(t *testing.T) {
		_, resp := readOrgMemberData(t, "nobody@example.com", http.StatusNotFound,
			`{"title": "Not Found", "status": 404, "detail": "not found", "code": "not_found"}`)
		require.True(t, resp.Diagnostics.HasError())
		require.Equal(t, "org_member not found", resp.Diagnostics[0].Summary())
		require.Contains(t, resp.Diagnostics[0].Detail(), "SSO provider")
	})
}
