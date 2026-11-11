package project

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
	str2duration "github.com/xhit/go-str2duration/v2"

	"github.com/uptrace/terraform/api"
)

// durationMs parses a duration string with the same library the resource
// uses and returns it as whole milliseconds — the wire representation the
// server expects.
func durationMs(t *testing.T, s string) int64 {
	t.Helper()
	d, err := str2duration.ParseDuration(s)
	require.NoError(t, err)
	return d.Milliseconds()
}

func projectResourceTestSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()

	var resp resource.SchemaResponse
	(&ProjectResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	return resp
}

func projectResourceTestState(t *testing.T, m projectModel) tfsdk.State {
	t.Helper()

	schemaResp := projectResourceTestSchema(t)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return state
}

func projectResourceTestPlan(t *testing.T, m projectModel) tfsdk.Plan {
	t.Helper()

	schemaResp := projectResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(context.Background(), &m)
	require.False(t, diags.HasError())

	return plan
}

func TestProjectToModel_fullPayload(t *testing.T) {
	p := &api.Project{
		ID:                      7,
		OrgID:                   42,
		Name:                    "api",
		GroupByEnv:              true,
		GroupFuncsByService:     false,
		SkipMonitorStatusEmails: true,
		SemconvVersion:          api.SemconvVersionV1330,
	}
	var m projectModel

	projectToModel(p, &m)

	require.Equal(t, "7", m.ID.ValueString())
	require.Equal(t, "42", m.OrgID.ValueString())
	require.Equal(t, "api", m.Name.ValueString())
	require.True(t, m.GroupByEnv.ValueBool())
	require.False(t, m.GroupFuncsByService.ValueBool())
	require.Equal(t, "v1.33.0", m.SemconvVersion.ValueString())
	require.True(t, m.SkipMonitorStatusEmails.ValueBool())
	require.False(t, m.SkipMonitorDisabledEmails.ValueBool())
}

func TestProjectToModel_retentionAndTimeRangeNotOverwritten(t *testing.T) {
	m := projectModel{
		SpanRetention:   types.StringValue("720h"),
		MetricRetention: types.StringValue("720h"),
		SpanTimeRange:   types.StringValue("24h"),
	}
	apiResp := &api.Project{
		ID:              1,
		Name:            "api",
		SpanRetention:   1_209_600_000, // 336h — the CE override
		MetricRetention: 1_209_600_000,
		SpanTimeRange:   0,
	}

	projectToModel(apiResp, &m)

	require.Equal(t, "720h", m.SpanRetention.ValueString(),
		"retention must not be overwritten by the API response")
	require.Equal(t, "720h", m.MetricRetention.ValueString())
	require.Equal(t, "24h", m.SpanTimeRange.ValueString())
}

func TestProjectRequestFields_retentionConvertedToMillis(t *testing.T) {
	m := &projectModel{
		Name:           types.StringValue("api"),
		SpanRetention:  types.StringValue("30d"), // str2duration-only unit
		EventRetention: types.StringValue("0s"),
	}

	body, err := projectRequestFields(m)
	require.NoError(t, err)

	require.NotNil(t, body.SpanRetention)
	require.Equal(t, durationMs(t, "30d"), *body.SpanRetention)
	// Explicit zero is passed through as the "use default" sentinel.
	require.NotNil(t, body.EventRetention)
	require.Equal(t, int64(0), *body.EventRetention)
	// Unset field is omitted.
	require.Nil(t, body.LogRetention)
}

func TestViewSettingsRequest_timeRangesConvertedToMillis(t *testing.T) {
	body, err := viewSettingsRequest(&projectModel{SpanTimeRange: types.StringValue("24h")})
	require.NoError(t, err)
	require.NotNil(t, body)
	require.NotNil(t, body.SpanTimeRange)
	require.Equal(t, durationMs(t, "24h"), *body.SpanTimeRange)
	require.Nil(t, body.LogTimeRange)
}

func TestViewSettingsRequest_noTimeRangeSendsNoPatch(t *testing.T) {
	body, err := viewSettingsRequest(&projectModel{SpanRetention: types.StringValue("30d")})
	require.NoError(t, err)
	require.Nil(t, body, "a config with no time range must not PATCH the view settings")
}

func TestProjectRequestFields_unsetFieldsAreOmitted(t *testing.T) {
	m := &projectModel{
		Name:       types.StringValue("api"),
		GroupByEnv: types.BoolValue(true),
	}

	body, err := projectRequestFields(m)
	require.NoError(t, err)

	require.Equal(t, "api", body.Name)
	require.NotNil(t, body.GroupByEnv)
	require.True(t, *body.GroupByEnv)
	require.Nil(t, body.GroupFuncsByService)
	require.Nil(t, body.SemconvVersion)
	require.Nil(t, body.SpanRetention)
	require.Nil(t, body.MetricRetention)
}

func TestProjectRequestFields_semconvEnumIsPassedThrough(t *testing.T) {
	m := &projectModel{
		Name:           types.StringValue("api"),
		SemconvVersion: types.StringValue("v1.25.0"),
	}

	body, err := projectRequestFields(m)
	require.NoError(t, err)

	require.NotNil(t, body.SemconvVersion)
	require.Equal(t, api.SemconvVersionV1250, *body.SemconvVersion)
}

func TestProjectImportState_rejectsInvalidOrgID(t *testing.T) {
	var resp resource.ImportStateResponse

	(&ProjectResource{}).ImportState(context.Background(), resource.ImportStateRequest{ID: "abc:123"}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid org_id in import ID")
}

func TestProjectRead_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := projectResourceTestState(t, projectModel{
		ID:    types.StringValue("4294967296"),
		OrgID: types.StringValue("42"),
		Name:  types.StringValue("api"),
	})
	req := resource.ReadRequest{State: reqState}
	resp := resource.ReadResponse{State: reqState}

	(&ProjectResource{}).Read(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project ID")
}

func TestProjectUpdate_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := projectResourceTestState(t, projectModel{
		ID:    types.StringValue("4294967296"),
		OrgID: types.StringValue("42"),
		Name:  types.StringValue("api"),
	})
	reqPlan := projectResourceTestPlan(t, projectModel{
		ID:    types.StringValue("4294967296"),
		OrgID: types.StringValue("42"),
		Name:  types.StringValue("api"),
	})
	req := resource.UpdateRequest{State: reqState, Plan: reqPlan}
	resp := resource.UpdateResponse{State: reqState}

	(&ProjectResource{}).Update(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project ID")
}

func TestProjectDelete_rejectsProjectIDOutsideUint32(t *testing.T) {
	ctx := context.Background()
	reqState := projectResourceTestState(t, projectModel{
		ID:    types.StringValue("4294967296"),
		OrgID: types.StringValue("42"),
		Name:  types.StringValue("api"),
	})
	req := resource.DeleteRequest{State: reqState}
	resp := resource.DeleteResponse{State: reqState}

	(&ProjectResource{}).Delete(ctx, req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "invalid project ID")
}
