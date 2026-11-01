package notifchan

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/tfutil"
)

var channelPriorities = []string{"info", "low", "medium", "high"}

// sharedAttributes returns the schema attributes common to every typed
// notification-channel resource. Each concrete resource appends its own
// type-specific params attributes.
func sharedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "Channel ID.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			Required:    true,
			Description: "Project ID this channel belongs to. Changing this forces recreation.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			Required:    true,
			Description: "Human-readable channel name.",
		},
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "Channel status (delivering, paused, disabled, draft).",
		},
		"match_all": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
			Description: "Match all monitors. If false, monitor_ids must be provided.",
		},
		"condition": schema.StringAttribute{
			Optional:    true,
			Description: "Alert condition expression.",
		},
		"priorities": schema.ListAttribute{
			Required:    true,
			ElementType: types.StringType,
			Description: "Alert priorities to match. Valid values: info, low, medium, high.",
			Validators: []validator.List{
				listvalidator.ValueStringsAre(stringvalidator.OneOf(channelPriorities...)),
			},
		},
		"monitor_ids": schema.SetAttribute{
			Optional:    true,
			ElementType: types.StringType,
			Description: "Monitor IDs to match when match_all is false. Omit to leave the links alone, as when monitors name the channel in their channel_ids; set [] to detach every monitor.",
		},
	}
}

// sharedFields is a pointer bundle each concrete-resource apply uses to write
// shared fields from the API response into its flat model.
type sharedFields struct {
	ID         *types.String
	Name       *types.String
	Status     *types.String
	MatchAll   *types.Bool
	Condition  *types.String
	Priorities *types.List
	MonitorIDs *types.Set
}

// applyShared writes channel fields common to every type from ch into f.
// Priorities always round-trips; MonitorIDs preserves an explicit empty list.
func applyShared(ctx context.Context, ch *channelResponse, f sharedFields) diag.Diagnostics {
	var diags diag.Diagnostics

	*f.ID = types.StringValue(strconv.FormatUint(ch.ID, 10))
	*f.Name = types.StringValue(ch.Name)
	*f.Status = types.StringValue(string(ch.Status))
	*f.MatchAll = types.BoolValue(ch.MatchAll)

	if ch.Condition != "" {
		*f.Condition = types.StringValue(ch.Condition)
	} else {
		*f.Condition = types.StringNull()
	}

	if len(ch.Priorities) > 0 {
		vals := make([]types.String, len(ch.Priorities))
		for i, p := range ch.Priorities {
			vals[i] = types.StringValue(string(p))
		}
		v, d := types.ListValueFrom(ctx, types.StringType, vals)
		diags.Append(d...)
		*f.Priorities = v
	} else {
		v, d := types.ListValueFrom(ctx, types.StringType, []types.String{})
		diags.Append(d...)
		*f.Priorities = v
	}

	// An unset monitor_ids does not manage the links (see buildSharedRequest),
	// so it stays null whatever links the monitors declared.
	if !f.MonitorIDs.IsNull() {
		vals := make([]types.String, len(ch.MonitorIds))
		for i, id := range ch.MonitorIds {
			vals[i] = types.StringValue(strconv.FormatUint(id, 10))
		}
		v, d := types.SetValueFrom(ctx, types.StringType, vals)
		diags.Append(d...)
		*f.MonitorIDs = v
	}

	return diags
}

// buildSharedRequest populates the non-params fields of a channel request
// from the shared pointer bundle. Callers set Type and Params themselves.
func buildSharedRequest(ctx context.Context, f sharedFields, body *channelRequest) diag.Diagnostics {
	var diags diag.Diagnostics

	body.Name = f.Name.ValueString()

	if !f.MatchAll.IsNull() && !f.MatchAll.IsUnknown() {
		v := f.MatchAll.ValueBool()
		body.MatchAll = &v
	}
	if !f.Condition.IsNull() && !f.Condition.IsUnknown() {
		v := f.Condition.ValueString()
		body.Condition = &v
	}

	var priorities []types.String
	diags.Append(f.Priorities.ElementsAs(ctx, &priorities, false)...)
	if diags.HasError() {
		return diags
	}
	for _, p := range priorities {
		body.Priorities = append(body.Priorities, generated.AlertPriority(p.ValueString()))
	}

	// A channel and a monitor share their links, and either side may declare
	// them. An unset monitor_ids sends null, which keeps the stored links, so a
	// monitor's channel_ids can own them; a set one replaces them, [] included.
	if !f.MonitorIDs.IsNull() && !f.MonitorIDs.IsUnknown() {
		body.MonitorIds = []uint64{}
		var monitorIDs []types.String
		diags.Append(f.MonitorIDs.ElementsAs(ctx, &monitorIDs, false)...)
		if diags.HasError() {
			return diags
		}
		for _, id := range monitorIDs {
			v, err := strconv.ParseUint(id.ValueString(), 10, 64)
			if err != nil {
				diags.AddAttributeError(
					path.Root("monitor_ids"),
					"invalid monitor_id",
					fmt.Sprintf("expected a decimal integer, got %q: %s", id.ValueString(), err.Error()),
				)
				return diags
			}
			body.MonitorIds = append(body.MonitorIds, v)
		}
	}

	return diags
}

// validateMatchAllRule enforces "match_all=false requires monitor_ids non-empty".
// Safe to call before the type is known — it only inspects match_all/monitor_ids.
func validateMatchAllRule(matchAll types.Bool, monitorIDs types.Set, diags *diag.Diagnostics) {
	if matchAll.IsNull() || matchAll.IsUnknown() || matchAll.ValueBool() {
		return
	}
	if monitorIDs.IsUnknown() {
		return
	}
	if monitorIDs.IsNull() || len(monitorIDs.Elements()) == 0 {
		diags.AddAttributeError(
			path.Root("monitor_ids"),
			"monitor_ids is required",
			"monitor_ids must be set and non-empty when match_all is false.",
		)
	}
}

// rejectChannelType guards a per-type read mapper against a stale state or
// an out-of-band type change. Returns a diagnostic when the API response
// does not match the expected type.
func rejectChannelType(expected string, ch *channelResponse, diags *diag.Diagnostics) {
	if ch.Type == expected {
		return
	}
	diags.AddError(
		"channel type mismatch",
		"expected notification channel of type "+expected+", but channel id="+strconv.FormatUint(ch.ID, 10)+" is type "+ch.Type+".",
	)
}

// valueStringToPtr returns nil for null/unknown, else a pointer to the
// underlying string. Used when building API requests where an absent
// optional field must serialize as "omitted", not "present and empty".
func valueStringToPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// importChannel accepts "<project_id>:<channel_id>". It sets monitor_ids to
// an empty set, so the first read fills in the links the channel has.
func importChannel(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tfutil.ImportStateCompoundID(ctx, req, resp,
		tfutil.ImportField{Name: "project_id", Parse: tfutil.ParseUint32},
		tfutil.ImportField{Name: "channel_id", Parse: tfutil.ParseUint64},
	)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monitor_ids"), types.SetValueMust(types.StringType, nil))...)
}
