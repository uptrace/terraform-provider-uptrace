package project

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/uptrace/terraform/generated"
)

func projectTokenResourceTestSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()

	var resp resource.SchemaResponse
	(&ProjectTokenResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	return resp
}

func projectTokenResourceTestState(t *testing.T, m projectTokenModel) tfsdk.State {
	t.Helper()

	schemaResp := projectTokenResourceTestSchema(t)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return state
}

func projectTokenResourceTestPlan(t *testing.T, m projectTokenModel) tfsdk.Plan {
	t.Helper()

	schemaResp := projectTokenResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return plan
}

func TestProjectTokenToModel_fullPayload(t *testing.T) {
	token := &generated.ProjectToken{
		ID:        42,
		ProjectID: 7,
		Name:      "ci-ingest",
		Token:     "secret123",
		Dsn:       "http://secret123@localhost:14318/7",
	}
	m := projectTokenModel{ProjectID: types.StringValue("7")}

	projectTokenToModel(token, &m)

	require.Equal(t, types.StringValue("42"), m.ID)
	require.Equal(t, types.StringValue("7"), m.ProjectID, "ProjectID must not be modified by the mapper")
	require.Equal(t, types.StringValue("ci-ingest"), m.Name)
	require.Equal(t, types.StringValue("secret123"), m.Token)
	require.Equal(t, types.StringValue("http://secret123@localhost:14318/7"), m.DSN)
}

func TestProjectTokenToModel_doesNotSetProjectID(t *testing.T) {
	token := &generated.ProjectToken{
		ID:        42,
		ProjectID: 7,
		Token:     "secret123",
	}
	m := projectTokenModel{ProjectID: types.StringValue("99")}

	projectTokenToModel(token, &m)

	require.Equal(t, types.StringValue("99"), m.ProjectID,
		"mapper must not overwrite ProjectID — callers own that field")
}

func TestProjectTokenToModel_dsnFollowsAPI(t *testing.T) {
	token := &generated.ProjectToken{
		ID:        42,
		ProjectID: 7,
		Token:     "secret123",
		Name:      "test",
		Dsn:       "https://secret123@uptrace.example.com?grpc=4317",
	}
	var m projectTokenModel

	projectTokenToModel(token, &m)

	require.Equal(t, types.StringValue("42"), m.ID)
	require.Equal(t, types.StringValue("secret123"), m.Token)
	require.Equal(t, types.StringValue("test"), m.Name)
	require.Equal(t, types.StringValue("https://secret123@uptrace.example.com?grpc=4317"), m.DSN)
}

func TestProjectTokenImportState_rejectsInvalidTokenID(t *testing.T) {
	var resp resource.ImportStateResponse

	(&ProjectTokenResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: "7:not-a-token"}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid token_id in import ID")
}

func TestProjectTokenCreate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqPlan := projectTokenResourceTestPlan(t, projectTokenModel{
		ProjectID: types.StringValue("4294967296"),
		Name:      types.StringValue("ci-ingest"),
	})
	req := resource.CreateRequest{Plan: reqPlan}
	resp := resource.CreateResponse{}

	(&ProjectTokenResource{}).Create(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestProjectTokenRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := projectTokenResourceTestState(t, projectTokenModel{
		ID:        types.StringValue("1"),
		ProjectID: types.StringValue("4294967296"),
		Name:      types.StringValue("ci-ingest"),
		Token:     types.StringValue("secret"),
		DSN:       types.StringValue("http://secret@localhost:14318/1"),
	})
	req := resource.ReadRequest{State: reqState}
	resp := resource.ReadResponse{State: reqState}

	(&ProjectTokenResource{}).Read(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestProjectTokenUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	model := projectTokenModel{
		ID:        types.StringValue("1"),
		ProjectID: types.StringValue("4294967296"),
		Name:      types.StringValue("ci-ingest"),
		Token:     types.StringValue("secret"),
		DSN:       types.StringValue("http://secret@localhost:14318/1"),
	}
	reqState := projectTokenResourceTestState(t, model)
	reqPlan := projectTokenResourceTestPlan(t, model)
	req := resource.UpdateRequest{State: reqState, Plan: reqPlan}
	resp := resource.UpdateResponse{State: reqState}

	(&ProjectTokenResource{}).Update(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}

func TestProjectTokenDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := projectTokenResourceTestState(t, projectTokenModel{
		ID:        types.StringValue("1"),
		ProjectID: types.StringValue("4294967296"),
		Name:      types.StringValue("ci-ingest"),
		Token:     types.StringValue("secret"),
		DSN:       types.StringValue("http://secret@localhost:14318/1"),
	})
	req := resource.DeleteRequest{State: reqState}
	resp := resource.DeleteResponse{State: reqState}

	(&ProjectTokenResource{}).Delete(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project_id")
}
