package notifchan

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/tfutil"
)

// channelCRUD bundles the per-type hooks that vary across channel resources.
// The shared CRUD helpers drive every backend call through it so each
// typed resource only provides its schema, model, builder, and mapper.
type channelCRUD[M any] struct {
	TypeName  string // e.g. "slack_channel" — only used in log/error messages
	ProjectID func(*M) types.String
	ID        func(*M) types.String
	Name      func(*M) types.String
	Build     func(context.Context, *M) (*channelRequest, diag.Diagnostics)
	Apply     func(context.Context, *channelResponse, *M) diag.Diagnostics
}

// channelRequest is a channel create or update body: the fields every type
// shares, the type, and that type's params. Each resource fills Params with
// its generated params struct, so the body names only keys the spec lists.
//
// The shared fields are spelled here rather than taken from
// api.BaseNotificationChannelRequest, which holds matchAll and condition as
// values. The body must tell an omitted key from false and from the empty
// string, and the members of api.NotificationChannelRequest, which the body
// is decoded into, hold those fields as pointers.
type channelRequest struct {
	Name       string              `json:"name"`
	MatchAll   *bool               `json:"matchAll,omitzero"`
	MonitorIDs *[]uint64           `json:"monitorIds,omitzero"`
	Condition  *string             `json:"condition,omitzero"`
	Priorities []api.AlertPriority `json:"priorities"`
	Type       string              `json:"type"`
	Params     any                 `json:"params"`
}

// channelResponse is a channel as the server returns it: the shared fields,
// the type, and the type's params left raw for the resource to decode.
type channelResponse struct {
	api.BaseNotificationChannel
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// toAPI converts the body to the generated union through its JSON form, so
// one builder serves every transport.
func (r *channelRequest) toAPI() (*api.NotificationChannelRequest, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	out := new(api.NotificationChannelRequest)
	if err := json.Unmarshal(b, out); err != nil {
		return nil, err
	}
	return out, nil
}

// channelFromAPI decodes the union the server answered with.
func channelFromAPI(ch *api.NotificationChannel) (*channelResponse, error) {
	b, err := json.Marshal(ch)
	if err != nil {
		return nil, err
	}
	out := new(channelResponse)
	if err := json.Unmarshal(b, out); err != nil {
		return nil, err
	}
	return out, nil
}

// applyAPI decodes ch and hands it to the resource's Apply hook.
func applyAPI[M any](ctx context.Context, h channelCRUD[M], ch *api.NotificationChannel, m *M) diag.Diagnostics {
	var diags diag.Diagnostics
	resp, err := channelFromAPI(ch)
	if err != nil {
		diags.AddError("failed to decode "+h.TypeName, err.Error())
		return diags
	}
	diags.Append(h.Apply(ctx, resp, m)...)
	return diags
}

// buildAPI runs the resource's Build hook and converts its body.
func buildAPI[M any](ctx context.Context, h channelCRUD[M], m *M) (*api.NotificationChannelRequest, diag.Diagnostics) {
	body, diags := h.Build(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	out, err := body.toAPI()
	if err != nil {
		diags.AddError("failed to encode "+h.TypeName, err.Error())
		return nil, diags
	}
	return out, diags
}

func channelCreate[M any](ctx context.Context, c *client.Client, plan *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(plan).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)

	body, d := buildAPI(ctx, h, plan)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	tflog.Info(ctx, "creating "+h.TypeName, map[string]any{
		"project_id": h.ProjectID(plan).ValueString(),
		"name":       h.Name(plan).ValueString(),
	})

	out, err := c.API.CreateNotificationChannel(ctx, api.CreateNotificationChannelRequest{ProjectID: projectID}, *body)
	if err != nil {
		tfutil.AddAPIError(&diags, "create "+h.TypeName+" failed", err)
		return diags
	}

	diags.Append(applyAPI(ctx, h, &out.Channel, plan)...)
	return diags
}

// channelRead returns removeResource=true when the backend reports the
// resource is gone (404 / 403). The caller should remove it from state.
func channelRead[M any](ctx context.Context, c *client.Client, state *M, h channelCRUD[M]) (removeResource bool, diags diag.Diagnostics) {
	projectID64, err := strconv.ParseUint(h.ProjectID(state).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return false, diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseUint(h.ID(state).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return false, diags
	}

	out, err := c.API.GetNotificationChannel(ctx, api.GetNotificationChannelRequest{ProjectID: projectID, ChannelID: channelID})
	if err != nil {
		if client.IsNotFound(err) {
			return true, diags
		}
		tfutil.AddAPIError(&diags, "read "+h.TypeName+" failed", err)
		return false, diags
	}

	diags.Append(applyAPI(ctx, h, &out.Channel, state)...)
	return false, diags
}

func channelUpdate[M any](ctx context.Context, c *client.Client, plan *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(plan).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseUint(h.ID(plan).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return diags
	}

	body, d := buildAPI(ctx, h, plan)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	tflog.Info(ctx, "updating "+h.TypeName, map[string]any{"id": h.ID(plan).ValueString()})

	out, err := c.API.UpdateNotificationChannel(ctx, api.UpdateNotificationChannelRequest{ProjectID: projectID, ChannelID: channelID}, *body)
	if err != nil {
		tfutil.AddAPIError(&diags, "update "+h.TypeName+" failed", err)
		return diags
	}

	diags.Append(applyAPI(ctx, h, &out.Channel, plan)...)
	return diags
}

func channelDelete[M any](ctx context.Context, c *client.Client, state *M, h channelCRUD[M]) diag.Diagnostics {
	var diags diag.Diagnostics

	projectID64, err := strconv.ParseUint(h.ProjectID(state).ValueString(), 10, 32)
	if err != nil {
		diags.AddError("invalid project_id", err.Error())
		return diags
	}
	projectID := uint32(projectID64)
	channelID, err := strconv.ParseUint(h.ID(state).ValueString(), 10, 64)
	if err != nil {
		diags.AddError("invalid channel ID", err.Error())
		return diags
	}

	tflog.Info(ctx, "deleting "+h.TypeName, map[string]any{"id": h.ID(state).ValueString()})

	_, err = c.API.DeleteNotificationChannel(ctx, api.DeleteNotificationChannelRequest{ProjectID: projectID, ChannelID: channelID})
	if err != nil && !client.IsNotFound(err) {
		tfutil.AddAPIError(&diags, "delete "+h.TypeName+" failed", err)
	}
	return diags
}
