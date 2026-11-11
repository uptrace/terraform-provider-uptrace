package monitor

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
)

var metricAliasRegexp = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)

// kinds lists every monitor type the provider manages, one resource each.
var kinds = []*kind{
	thresholdKind,
	anomalyKind,
	outlierKind,
	forecastKind,
	errorKind,
	sloKind,
}

// --- schema helpers ---

func optionalComputedEnum[T ~string](desc string, values ...T) schema.StringAttribute {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = string(v)
	}
	return schema.StringAttribute{
		Optional:      true,
		Computed:      true,
		Description:   desc + " Omit for the server default.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		Validators:    []validator.String{stringvalidator.OneOf(strs...)},
	}
}

// durationAttribute is a duration string, such as "5m" or "1d". A computed
// one takes the server default when the config omits it.
func durationAttribute(computed bool, desc string) schema.StringAttribute {
	a := schema.StringAttribute{
		Optional:    true,
		Description: desc + ` A duration string: ms, s, m, h, d (day) or w (week).`,
		Validators:  []validator.String{durationValidator{}},
	}
	if computed {
		a.Computed = true
		a.PlanModifiers = []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	}
	return a
}

func optionalComputedInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:      true,
		Computed:      true,
		Description:   desc + " Omit for the server default.",
		PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}

// Bounds of the knobs whose zero the server rewrites to a default. Each
// mirrors a constant of the app's alerting/monitorspec, so a value the server
// would not store as sent fails the plan.
const (
	minNumEvalPoints     = 1                   // 1, the low bound of monitorspec.ValidateNumEvalPoints
	defaultNumEvalPoints = 5                   // monitorspec.DefaultNumEvalPoints, the width when num_eval_points is unset
	maxNumEvalPoints     = 15                  // monitorspec.MaxNumEvalPoints
	minOutlierMinPeers   = 3                   // monitorspec.MinOutlierMinPeers
	maxOutlierMinPeers   = 10000               // monitorspec.MaxOutlierMinPeers
	minOutlierKeepFiring = 10 * time.Second    // monitorspec.MinOutlierKeepFiringFor
	maxOutlierKeepFiring = 2 * time.Hour       // monitorspec.MaxOutlierKeepFiringFor
	minForecastHorizon   = time.Hour           // monitorspec.MinForecastHorizon
	maxForecastHorizon   = 14 * 24 * time.Hour // monitorspec.MaxForecastHorizon
)

// boundedInt is an optionalComputedInt held to [min, max].
func boundedInt(desc string, min, max int64) schema.Int64Attribute {
	a := optionalComputedInt(desc)
	a.Validators = []validator.Int64{int64validator.Between(min, max)}
	return a
}

// boundedDuration is a computed durationAttribute held to [min, max].
func boundedDuration(desc string, min, max time.Duration) schema.StringAttribute {
	a := durationAttribute(true, desc)
	a.Validators = append(a.Validators, durationBetween{min: min, max: max})
	return a
}

func optionalComputedFloat(desc string) schema.Float64Attribute {
	return schema.Float64Attribute{
		Optional:      true,
		Computed:      true,
		Description:   desc + " Omit for the server default.",
		PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
	}
}

// --- metrics and the params every metric-based kind shares ---

type metricAliasModel struct {
	Name  types.String `tfsdk:"name"`
	Alias types.String `tfsdk:"alias"`
}

func metricsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Required:    true,
		Description: "Metrics the query refers to, each under an alias such as $requests.",
		Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{Required: true, Description: "Metric name."},
				"alias": schema.StringAttribute{
					Required:    true,
					Description: "Alias the query uses: $ followed by an identifier.",
					Validators: []validator.String{
						stringvalidator.RegexMatches(metricAliasRegexp, "must start with $ followed by an identifier"),
					},
				},
			},
		},
	}
}

func metricsToAPI(ms []metricAliasModel) []api.MetricAlias {
	out := make([]api.MetricAlias, len(ms))
	for i, m := range ms {
		out[i] = api.MetricAlias{Name: m.Name.ValueString(), Alias: m.Alias.ValueString()}
	}
	return out
}

func metricsFromAPI(ms []api.MetricAlias) []metricAliasModel {
	out := make([]metricAliasModel, len(ms))
	for i, m := range ms {
		out[i] = metricAliasModel{Name: types.StringValue(m.Name), Alias: types.StringValue(m.Alias)}
	}
	return out
}

var columnAttrTypes = map[string]attr.Type{"name": types.StringType, "unit": types.StringType}

// metricParamsModel holds the params of every metric-based kind: what to
// query and how to evaluate it. Each kind's model embeds it.
type metricParamsModel struct {
	Metrics         []metricAliasModel `tfsdk:"metrics"`
	Query           types.String       `tfsdk:"query"`
	Column          types.Object       `tfsdk:"column"`
	Resolution      types.String       `tfsdk:"resolution"`
	NumEvalPoints   types.Int64        `tfsdk:"num_eval_points"`
	WindowRule      types.String       `tfsdk:"window_rule"`
	MinBreachPoints types.Int64        `tfsdk:"min_breach_points"`
	AbsentPoints    types.String       `tfsdk:"absent_points"`
	TimeOffset      types.String       `tfsdk:"time_offset"`
}

// metricParamsJSON is metricParamsModel on the wire. Each kind's params
// struct embeds it, and encoding/json flattens it into the same object.
type metricParamsJSON struct {
	Metrics         []api.MetricAlias  `json:"metrics"`
	Query           string             `json:"query"`
	Column          *api.MonitorColumn `json:"column,omitempty"`
	Resolution      *int64             `json:"resolution,omitempty"`
	NumEvalPoints   *int64             `json:"numEvalPoints,omitempty"`
	WindowRule      *string            `json:"windowRule,omitempty"`
	MinBreachPoints *int64             `json:"minBreachPoints,omitempty"`
	AbsentPoints    *string            `json:"absentPoints,omitempty"`
	TimeOffset      *int64             `json:"timeOffset,omitempty"`
}

func metricParamsAttributes(extra map[string]schema.Attribute) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"metrics": metricsAttribute(),
		"query": schema.StringAttribute{
			Required:    true,
			Description: "MQL expression that produces the monitored value, such as `avg($cpu) | group by host_name`.",
		},
		"column": schema.SingleNestedAttribute{
			Optional:      true,
			Computed:      true,
			Description:   "The query result column the monitor evaluates, and its display unit. Omit to evaluate the first column.",
			PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{Required: true, Description: "Column name, as the query names it."},
				"unit": schema.StringAttribute{
					Optional:      true,
					Computed:      true,
					Description:   "Display unit, such as milliseconds, bytes or utilization.",
					PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				},
			},
		},
		"resolution":      durationAttribute(true, "Width of one evaluated point."),
		"num_eval_points": boundedInt("How many recent points each check evaluates.", minNumEvalPoints, maxNumEvalPoints),
		"window_rule": optionalComputedEnum("How the evaluated points decide a breach.",
			api.MetricWindowRuleAtAllTimes, api.MetricWindowRuleAtLeast,
			api.MetricWindowRuleOnAverage, api.MetricWindowRuleInTotal),
		"min_breach_points": optionalComputedInt("Breaching points the at_least window rule needs. A count of 1 or more with no window_rule means at_least."),
		"absent_points": optionalComputedEnum("How a point with no data counts: zero, ignore, or alert (a missing point breaches).",
			api.AbsentPointsZero, api.AbsentPointsIgnore, api.AbsentPointsAlert),
		"time_offset": durationAttribute(true, "How far back the evaluated window ends, for data that arrives late."),
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return attrs
}

func (m *metricParamsModel) toAPI(ctx context.Context, diags *diag.Diagnostics) metricParamsJSON {
	out := metricParamsJSON{
		Metrics:         metricsToAPI(m.Metrics),
		Query:           m.Query.ValueString(),
		NumEvalPoints:   int64Ptr(m.NumEvalPoints),
		WindowRule:      stringPtr(m.WindowRule),
		MinBreachPoints: int64Ptr(m.MinBreachPoints),
		AbsentPoints:    stringPtr(m.AbsentPoints),
	}
	if !m.Column.IsNull() && !m.Column.IsUnknown() {
		var col struct {
			Name types.String `tfsdk:"name"`
			Unit types.String `tfsdk:"unit"`
		}
		diags.Append(m.Column.As(ctx, &col, basetypes.ObjectAsOptions{UnhandledUnknownAsEmpty: true})...)
		out.Column = &api.MonitorColumn{Name: col.Name.ValueString(), Unit: col.Unit.ValueString()}
	}
	var err error
	if out.Resolution, err = durationMillis(m.Resolution); err != nil {
		diags.AddError("invalid params.resolution", err.Error())
	}
	if out.TimeOffset, err = durationMillis(m.TimeOffset); err != nil {
		diags.AddError("invalid params.time_offset", err.Error())
	}
	return out
}

func (m *metricParamsModel) fromAPI(ctx context.Context, p *metricParamsJSON, prior *metricParamsModel, diags *diag.Diagnostics) {
	m.Metrics = metricsFromAPI(p.Metrics)
	m.Query = stringOrPrior(p.Query, prior.Query)

	col := api.MonitorColumn{}
	if p.Column != nil {
		col = *p.Column
	}
	v, d := types.ObjectValue(columnAttrTypes, map[string]attr.Value{
		"name": types.StringValue(col.Name),
		"unit": types.StringValue(col.Unit),
	})
	diags.Append(d...)
	m.Column = v

	m.Resolution = optionalDuration(p.Resolution, prior.Resolution)
	m.NumEvalPoints = types.Int64PointerValue(p.NumEvalPoints)
	m.WindowRule = types.StringPointerValue(p.WindowRule)
	m.MinBreachPoints = types.Int64PointerValue(p.MinBreachPoints)
	m.AbsentPoints = types.StringPointerValue(p.AbsentPoints)
	m.TimeOffset = optionalDuration(p.TimeOffset, prior.TimeOffset)
}

// --- generic params plumbing ---

// paramsFromObject reads a params attribute into its model.
func paramsFromObject[M any](ctx context.Context, obj types.Object, diags *diag.Diagnostics) *M {
	m := new(M)
	if obj.IsNull() || obj.IsUnknown() {
		return m
	}
	diags.Append(obj.As(ctx, m, basetypes.ObjectAsOptions{UnhandledUnknownAsEmpty: true})...)
	return m
}

// paramsToObject writes a params model as the attribute value.
func paramsToObject[M any](ctx context.Context, k *kind, m *M, diags *diag.Diagnostics) types.Object {
	v, d := types.ObjectValueFrom(ctx, k.paramsAttrTypes(), m)
	diags.Append(d...)
	return v
}

// sendAs decodes body into the kind's generated request and sends it. The
// generated request is the spec's key set, so an encoding mistake here fails
// in the decode instead of reaching the server.
func sendAs[B any](body []byte, send func(*B) (*api.MonitorSummaryResponse, error)) (*api.MonitorSummaryResponse, error) {
	b := new(B)
	if err := json.Unmarshal(body, b); err != nil {
		return nil, err
	}
	return send(b)
}

// --- threshold ---

type thresholdParamsModel struct {
	metricParamsModel
	MinValue types.Float64 `tfsdk:"min_value"`
	MaxValue types.Float64 `tfsdk:"max_value"`
	Recovery types.Object  `tfsdk:"recovery"`
}

type thresholdParamsJSON struct {
	metricParamsJSON
	MinValue *float64             `json:"minValue,omitempty"`
	MaxValue *float64             `json:"maxValue,omitempty"`
	Recovery *api.MonitorRecovery `json:"recovery,omitempty"`
}

var recoveryAttrTypes = map[string]attr.Type{"min_value": types.Float64Type, "max_value": types.Float64Type}

var thresholdKind = &kind{
	name:        "threshold",
	description: "Manages an Uptrace threshold monitor: it alerts when the monitored value leaves a fixed range.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "What the monitor queries and the range it holds the value to. Set min_value, max_value or both.",
		Attributes: metricParamsAttributes(map[string]schema.Attribute{
			"min_value": schema.Float64Attribute{Optional: true, Description: "The monitor fires when the value drops below it."},
			"max_value": schema.Float64Attribute{Optional: true, Description: "The monitor fires when the value rises above it."},
			"recovery": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The range a firing monitor must return to before it recovers. It must sit inside min_value..max_value.",
				Attributes: map[string]schema.Attribute{
					"min_value": schema.Float64Attribute{Optional: true, Description: "Lower recovery bound."},
					"max_value": schema.Float64Attribute{Optional: true, Description: "Upper recovery bound."},
				},
			},
		}),
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ThresholdMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateThresholdMonitor(ctx, api.CreateThresholdMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ThresholdMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateThresholdMonitor(ctx, api.UpdateThresholdMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	thresholdKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[thresholdParamsModel](ctx, obj, &diags)
		out := thresholdParamsJSON{
			metricParamsJSON: m.toAPI(ctx, &diags),
			MinValue:         float64Ptr(m.MinValue),
			MaxValue:         float64Ptr(m.MaxValue),
		}
		if !m.Recovery.IsNull() && !m.Recovery.IsUnknown() {
			var r struct {
				MinValue types.Float64 `tfsdk:"min_value"`
				MaxValue types.Float64 `tfsdk:"max_value"`
			}
			diags.Append(m.Recovery.As(ctx, &r, basetypes.ObjectAsOptions{})...)
			out.Recovery = &api.MonitorRecovery{MinValue: float64Ptr(r.MinValue), MaxValue: float64Ptr(r.MaxValue)}
		}
		return out, diags
	}
	thresholdKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p thresholdParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode threshold monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[thresholdParamsModel](ctx, priorObj, &diags)
		var m thresholdParamsModel
		m.fromAPI(ctx, &p.metricParamsJSON, &prior.metricParamsModel, &diags)
		m.MinValue = float64Value(p.MinValue)
		m.MaxValue = float64Value(p.MaxValue)
		m.Recovery = types.ObjectNull(recoveryAttrTypes)
		if p.Recovery != nil && (p.Recovery.MinValue != nil || p.Recovery.MaxValue != nil || !prior.Recovery.IsNull()) {
			v, d := types.ObjectValue(recoveryAttrTypes, map[string]attr.Value{
				"min_value": float64Value(p.Recovery.MinValue),
				"max_value": float64Value(p.Recovery.MaxValue),
			})
			diags.Append(d...)
			m.Recovery = v
		}
		return paramsToObject(ctx, thresholdKind, &m, &diags), diags
	}
}

// --- anomaly ---

type anomalyParamsModel struct {
	metricParamsModel
	Preset      types.String  `tfsdk:"preset"`
	Sensitivity types.String  `tfsdk:"sensitivity"`
	Direction   types.String  `tfsdk:"direction"`
	Seasonality types.String  `tfsdk:"seasonality"`
	MinVolume   types.Float64 `tfsdk:"min_volume"`
	MinChange   types.Float64 `tfsdk:"min_change"`
}

type anomalyParamsJSON struct {
	metricParamsJSON
	Preset      *string  `json:"preset,omitempty"`
	Sensitivity *string  `json:"sensitivity,omitempty"`
	Direction   *string  `json:"direction,omitempty"`
	Seasonality *string  `json:"seasonality,omitempty"`
	MinVolume   *float64 `json:"minVolume,omitempty"`
	MinChange   *float64 `json:"minChange,omitempty"`
}

func sensitivityAttribute() schema.StringAttribute {
	return optionalComputedEnum("How far a value must stray before it counts.",
		api.DetectorSensitivityVeryLow, api.DetectorSensitivityLow, api.DetectorSensitivityMedium,
		api.DetectorSensitivityHigh, api.DetectorSensitivityVeryHigh)
}

func directionAttribute() schema.StringAttribute {
	return optionalComputedEnum("Which deviations alert.",
		api.DirectionFilterAbove, api.DirectionFilterBelow, api.DirectionFilterBoth)
}

func seasonalityAttribute() schema.StringAttribute {
	return optionalComputedEnum("Whether the model learns daily and weekly cycles.",
		api.DetectorSeasonalityAuto, api.DetectorSeasonalityOff)
}

var anomalyKind = &kind{
	name:        "anomaly",
	description: "Manages an Uptrace anomaly monitor: it learns each series' normal range and alerts when the value leaves it.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "What the monitor queries and how its anomaly detector judges the value.",
		Attributes: metricParamsAttributes(map[string]schema.Attribute{
			"preset":      optionalComputedEnum("Detector preset.", api.AnomalyPresetGeneral),
			"sensitivity": sensitivityAttribute(),
			"direction":   directionAttribute(),
			"seasonality": seasonalityAttribute(),
			"min_volume":  optionalComputedFloat("Smallest value a series needs before it is judged."),
			"min_change":  optionalComputedFloat("Smallest absolute change that counts as an anomaly."),
		}),
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.AnomalyMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateAnomalyMonitor(ctx, api.CreateAnomalyMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.AnomalyMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateAnomalyMonitor(ctx, api.UpdateAnomalyMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	anomalyKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[anomalyParamsModel](ctx, obj, &diags)
		return anomalyParamsJSON{
			metricParamsJSON: m.toAPI(ctx, &diags),
			Preset:           stringPtr(m.Preset),
			Sensitivity:      stringPtr(m.Sensitivity),
			Direction:        stringPtr(m.Direction),
			Seasonality:      stringPtr(m.Seasonality),
			MinVolume:        float64Ptr(m.MinVolume),
			MinChange:        float64Ptr(m.MinChange),
		}, diags
	}
	anomalyKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p anomalyParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode anomaly monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[anomalyParamsModel](ctx, priorObj, &diags)
		var m anomalyParamsModel
		m.fromAPI(ctx, &p.metricParamsJSON, &prior.metricParamsModel, &diags)
		m.Preset = types.StringPointerValue(p.Preset)
		m.Sensitivity = types.StringPointerValue(p.Sensitivity)
		m.Direction = types.StringPointerValue(p.Direction)
		m.Seasonality = types.StringPointerValue(p.Seasonality)
		m.MinVolume = float64Value(p.MinVolume)
		m.MinChange = float64Value(p.MinChange)
		return paramsToObject(ctx, anomalyKind, &m, &diags), diags
	}
}

// --- outlier ---

type outlierParamsModel struct {
	metricParamsModel
	MinPeers      types.Int64   `tfsdk:"min_peers"`
	MinChange     types.Float64 `tfsdk:"min_change"`
	Sensitivity   types.String  `tfsdk:"sensitivity"`
	Direction     types.String  `tfsdk:"direction"`
	KeepFiringFor types.String  `tfsdk:"keep_firing_for"`
}

type outlierParamsJSON struct {
	metricParamsJSON
	MinPeers      *int64   `json:"minPeers,omitempty"`
	MinChange     *float64 `json:"minChange,omitempty"`
	Sensitivity   *string  `json:"sensitivity,omitempty"`
	Direction     *string  `json:"direction,omitempty"`
	KeepFiringFor *int64   `json:"keepFiringFor,omitempty"`
}

var outlierKind = &kind{
	name:        "outlier",
	description: "Manages an Uptrace outlier monitor: it alerts when one series strays from its peers, such as one host among many.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "What the monitor queries and how a series is judged against its peers.",
		Attributes: metricParamsAttributes(map[string]schema.Attribute{
			"min_peers":       boundedInt("Fewest series a group needs before any is judged.", minOutlierMinPeers, maxOutlierMinPeers),
			"min_change":      optionalComputedFloat("Smallest absolute distance from the peers that counts."),
			"sensitivity":     sensitivityAttribute(),
			"direction":       directionAttribute(),
			"keep_firing_for": boundedDuration("How long an alert stays open after the series rejoins its peers.", minOutlierKeepFiring, maxOutlierKeepFiring),
		}),
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.OutlierMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateOutlierMonitor(ctx, api.CreateOutlierMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.OutlierMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateOutlierMonitor(ctx, api.UpdateOutlierMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	outlierKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[outlierParamsModel](ctx, obj, &diags)
		out := outlierParamsJSON{
			metricParamsJSON: m.toAPI(ctx, &diags),
			MinPeers:         int64Ptr(m.MinPeers),
			MinChange:        float64Ptr(m.MinChange),
			Sensitivity:      stringPtr(m.Sensitivity),
			Direction:        stringPtr(m.Direction),
		}
		var err error
		if out.KeepFiringFor, err = durationMillis(m.KeepFiringFor); err != nil {
			diags.AddError("invalid params.keep_firing_for", err.Error())
		}
		return out, diags
	}
	outlierKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p outlierParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode outlier monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[outlierParamsModel](ctx, priorObj, &diags)
		var m outlierParamsModel
		m.fromAPI(ctx, &p.metricParamsJSON, &prior.metricParamsModel, &diags)
		m.MinPeers = types.Int64PointerValue(p.MinPeers)
		m.MinChange = float64Value(p.MinChange)
		m.Sensitivity = types.StringPointerValue(p.Sensitivity)
		m.Direction = types.StringPointerValue(p.Direction)
		m.KeepFiringFor = optionalDuration(p.KeepFiringFor, prior.KeepFiringFor)
		return paramsToObject(ctx, outlierKind, &m, &diags), diags
	}
}

// --- forecast ---

type forecastParamsModel struct {
	metricParamsModel
	MinValue    types.Float64 `tfsdk:"min_value"`
	MaxValue    types.Float64 `tfsdk:"max_value"`
	Seasonality types.String  `tfsdk:"seasonality"`
	Horizon     types.String  `tfsdk:"horizon"`
}

type forecastParamsJSON struct {
	metricParamsJSON
	MinValue    *float64 `json:"minValue,omitempty"`
	MaxValue    *float64 `json:"maxValue,omitempty"`
	Seasonality *string  `json:"seasonality,omitempty"`
	Horizon     *int64   `json:"horizon,omitempty"`
}

var forecastKind = &kind{
	name:        "forecast",
	description: "Manages an Uptrace forecast monitor: it alerts when the value is projected to leave a range within a horizon, such as a disk filling up.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "What the monitor queries, the range the forecast must stay in, and how far ahead it looks.",
		Attributes: metricParamsAttributes(map[string]schema.Attribute{
			"min_value":   schema.Float64Attribute{Optional: true, Description: "The monitor fires when the forecast drops below it."},
			"max_value":   schema.Float64Attribute{Optional: true, Description: "The monitor fires when the forecast rises above it."},
			"seasonality": seasonalityAttribute(),
			"horizon":     boundedDuration("How far ahead the forecast looks.", minForecastHorizon, maxForecastHorizon),
		}),
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ForecastMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateForecastMonitor(ctx, api.CreateForecastMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ForecastMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateForecastMonitor(ctx, api.UpdateForecastMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	forecastKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[forecastParamsModel](ctx, obj, &diags)
		out := forecastParamsJSON{
			metricParamsJSON: m.toAPI(ctx, &diags),
			MinValue:         float64Ptr(m.MinValue),
			MaxValue:         float64Ptr(m.MaxValue),
			Seasonality:      stringPtr(m.Seasonality),
		}
		var err error
		if out.Horizon, err = durationMillis(m.Horizon); err != nil {
			diags.AddError("invalid params.horizon", err.Error())
		}
		return out, diags
	}
	forecastKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p forecastParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode forecast monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[forecastParamsModel](ctx, priorObj, &diags)
		var m forecastParamsModel
		m.fromAPI(ctx, &p.metricParamsJSON, &prior.metricParamsModel, &diags)
		m.MinValue = float64Value(p.MinValue)
		m.MaxValue = float64Value(p.MaxValue)
		m.Seasonality = types.StringPointerValue(p.Seasonality)
		m.Horizon = optionalDuration(p.Horizon, prior.Horizon)
		return paramsToObject(ctx, forecastKind, &m, &diags), diags
	}
}

// --- error ---

type errorParamsModel struct {
	Metrics []metricAliasModel `tfsdk:"metrics"`
	Query   types.String       `tfsdk:"query"`
}

type errorParamsJSON struct {
	Metrics []api.MetricAlias `json:"metrics"`
	Query   string            `json:"query"`
}

var errorKind = &kind{
	name:        "error",
	description: "Manages an Uptrace error monitor: it alerts on each new group of error logs or error spans the query selects.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "The error-count query; each group it yields is one alert.",
		Attributes: map[string]schema.Attribute{
			"metrics": metricsAttribute(),
			"query": schema.StringAttribute{
				Required:    true,
				Description: `MQL expression, such as sum($logs) | group by _group_id | where _system in ("log:error", "log:fatal").`,
			},
		},
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ErrorMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateErrorMonitor(ctx, api.CreateErrorMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.ErrorMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateErrorMonitor(ctx, api.UpdateErrorMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	errorKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[errorParamsModel](ctx, obj, &diags)
		return errorParamsJSON{Metrics: metricsToAPI(m.Metrics), Query: m.Query.ValueString()}, diags
	}
	errorKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p errorParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode error monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[errorParamsModel](ctx, priorObj, &diags)
		m := errorParamsModel{Metrics: metricsFromAPI(p.Metrics), Query: stringOrPrior(p.Query, prior.Query)}
		return paramsToObject(ctx, errorKind, &m, &diags), diags
	}
}

// --- slo ---

type sloParamsModel struct {
	Metrics       []metricAliasModel `tfsdk:"metrics"`
	GoodQuery     types.String       `tfsdk:"good_query"`
	TotalQuery    types.String       `tfsdk:"total_query"`
	Grouping      types.List         `tfsdk:"grouping"`
	Resolution    types.String       `tfsdk:"resolution"`
	Target        types.Float64      `tfsdk:"target"`
	WindowKind    types.String       `tfsdk:"window_kind"`
	RollingWindow types.String       `tfsdk:"rolling_window"`
	Rules         types.List         `tfsdk:"rules"`
}

type sloRuleModel struct {
	Name          types.String  `tfsdk:"name"`
	Priority      types.String  `tfsdk:"priority"`
	Type          types.String  `tfsdk:"type"`
	LongWindow    types.String  `tfsdk:"long_window"`
	ShortWindow   types.String  `tfsdk:"short_window"`
	Factor        types.Float64 `tfsdk:"factor"`
	ConsumedBound types.Float64 `tfsdk:"consumed_bound"`
	MinTotal      types.Float64 `tfsdk:"min_total"`
}

type sloSliJSON struct {
	Type       string `json:"type"`
	GoodQuery  string `json:"goodQuery"`
	TotalQuery string `json:"totalQuery"`
}

// sloRuleConfigJSON holds the config of every rule type: burn_rate uses the
// windows and the factor, budget_consumed the bound; both use minTotal.
type sloRuleConfigJSON struct {
	LongWindow    *int64   `json:"longWindow,omitempty"`
	ShortWindow   *int64   `json:"shortWindow,omitempty"`
	Factor        *float64 `json:"factor,omitempty"`
	ConsumedBound *float64 `json:"consumedBound,omitempty"`
	MinTotal      float64  `json:"minTotal"`
}

type sloRuleJSON struct {
	Name     string            `json:"name"`
	Priority string            `json:"priority"`
	Type     string            `json:"type"`
	Config   sloRuleConfigJSON `json:"config"`
}

type sloParamsJSON struct {
	Metrics       []api.MetricAlias `json:"metrics"`
	Sli           sloSliJSON        `json:"sli"`
	Grouping      []string          `json:"grouping,omitempty"`
	Resolution    int64             `json:"resolution"`
	Target        float64           `json:"target"`
	WindowKind    *string           `json:"windowKind,omitempty"`
	RollingWindow int64             `json:"rollingWindow"`
	Rules         []sloRuleJSON     `json:"rules,omitempty"`
}

var sloRuleAttrTypes = map[string]attr.Type{
	"name":           types.StringType,
	"priority":       types.StringType,
	"type":           types.StringType,
	"long_window":    types.StringType,
	"short_window":   types.StringType,
	"factor":         types.Float64Type,
	"consumed_bound": types.Float64Type,
	"min_total":      types.Float64Type,
}

var sloKind = &kind{
	name:        "slo",
	description: "Manages an Uptrace SLO monitor: it tracks the share of good events against a target and alerts on its rules, such as a burn rate.",
	params: schema.SingleNestedAttribute{
		Required:    true,
		Description: "The SLI, the target and the alert rules.",
		Attributes: map[string]schema.Attribute{
			"metrics":     metricsAttribute(),
			"good_query":  schema.StringAttribute{Required: true, Description: "Selector that counts good events, such as histogram_count($spans{_status_code=\"ok\"})."},
			"total_query": schema.StringAttribute{Required: true, Description: "Selector that counts every event, such as histogram_count($spans)."},
			"grouping": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Expressions that split the SLO into one per group, such as service_name.",
			},
			"resolution": schema.StringAttribute{
				Required: true, Description: "Width of one evaluated point, such as 1m.",
				Validators: []validator.String{durationValidator{}},
			},
			"target": schema.Float64Attribute{Required: true, Description: "The good share the SLO promises, such as 0.999."},
			"window_kind": optionalComputedEnum("The budget window.",
				api.SLOBudgetWindowKindRolling, api.SLOBudgetWindowKindCalendarWeek,
				api.SLOBudgetWindowKindCalendarMonth, api.SLOBudgetWindowKindCalendarQuarter),
			"rolling_window": durationAttribute(true, "Length of a rolling budget window, such as 30d."),
			"rules": schema.ListNestedAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Alert rules. Omit for none.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{Required: true, Description: "Rule name."},
						"priority": schema.StringAttribute{
							Required: true, Description: "Priority of the alerts the rule opens: info, low, medium or high.",
							Validators: []validator.String{stringvalidator.OneOf("info", "low", "medium", "high")},
						},
						"type": schema.StringAttribute{
							Required: true, Description: "burn_rate or budget_consumed.",
							Validators: []validator.String{stringvalidator.OneOf("burn_rate", "budget_consumed")},
						},
						"long_window":    durationAttribute(false, "burn_rate: the long window."),
						"short_window":   durationAttribute(false, "burn_rate: the short window."),
						"factor":         schema.Float64Attribute{Optional: true, Description: "burn_rate: the burn rate that fires."},
						"consumed_bound": schema.Float64Attribute{Optional: true, Description: "budget_consumed: the consumed budget share that fires."},
						"min_total":      schema.Float64Attribute{Optional: true, Description: "Fewest events a window needs before the rule is judged."},
					},
				},
			},
		},
	},
	create: func(ctx context.Context, c *client.Client, projectID uint32, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.SLOMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.CreateSLOMonitor(ctx, api.CreateSLOMonitorRequest{ProjectID: projectID}, *b)
		})
	},
	update: func(ctx context.Context, c *client.Client, projectID uint32, id uint64, body []byte) (*api.MonitorSummaryResponse, error) {
		return sendAs(body, func(b *api.SLOMonitorRequest) (*api.MonitorSummaryResponse, error) {
			return c.API.UpdateSLOMonitor(ctx, api.UpdateSLOMonitorRequest{ProjectID: projectID, MonitorID: id}, *b)
		})
	},
}

func init() {
	sloKind.toAPI = func(ctx context.Context, obj types.Object) (any, diag.Diagnostics) {
		var diags diag.Diagnostics
		m := paramsFromObject[sloParamsModel](ctx, obj, &diags)
		out := sloParamsJSON{
			Metrics:    metricsToAPI(m.Metrics),
			Sli:        sloSliJSON{Type: "event", GoodQuery: m.GoodQuery.ValueString(), TotalQuery: m.TotalQuery.ValueString()},
			Target:     m.Target.ValueFloat64(),
			WindowKind: stringPtr(m.WindowKind),
		}
		if !m.Grouping.IsNull() && !m.Grouping.IsUnknown() {
			diags.Append(m.Grouping.ElementsAs(ctx, &out.Grouping, false)...)
		}
		if v, err := durationMillis(m.Resolution); err != nil {
			diags.AddError("invalid params.resolution", err.Error())
		} else if v != nil {
			out.Resolution = *v
		}
		if v, err := durationMillis(m.RollingWindow); err != nil {
			diags.AddError("invalid params.rolling_window", err.Error())
		} else if v != nil {
			out.RollingWindow = *v
		}
		if !m.Rules.IsNull() && !m.Rules.IsUnknown() {
			var rules []sloRuleModel
			diags.Append(m.Rules.ElementsAs(ctx, &rules, false)...)
			for _, r := range rules {
				rule := sloRuleJSON{
					Name: r.Name.ValueString(), Priority: r.Priority.ValueString(), Type: r.Type.ValueString(),
					Config: sloRuleConfigJSON{MinTotal: r.MinTotal.ValueFloat64()},
				}
				switch rule.Type {
				case "burn_rate":
					var err error
					if rule.Config.LongWindow, err = durationMillis(r.LongWindow); err != nil {
						diags.AddError("invalid params.rules.long_window", err.Error())
					}
					if rule.Config.ShortWindow, err = durationMillis(r.ShortWindow); err != nil {
						diags.AddError("invalid params.rules.short_window", err.Error())
					}
					f := r.Factor.ValueFloat64()
					rule.Config.Factor = &f
				case "budget_consumed":
					b := r.ConsumedBound.ValueFloat64()
					rule.Config.ConsumedBound = &b
				}
				out.Rules = append(out.Rules, rule)
			}
		}
		return out, diags
	}
	sloKind.fromAPI = func(ctx context.Context, raw json.RawMessage, priorObj types.Object) (types.Object, diag.Diagnostics) {
		var diags diag.Diagnostics
		var p sloParamsJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			diags.AddError("failed to decode SLO monitor params", err.Error())
			return priorObj, diags
		}
		prior := paramsFromObject[sloParamsModel](ctx, priorObj, &diags)
		var priorRules []sloRuleModel
		if !prior.Rules.IsNull() && !prior.Rules.IsUnknown() {
			diags.Append(prior.Rules.ElementsAs(ctx, &priorRules, false)...)
		}

		m := sloParamsModel{
			Metrics:       metricsFromAPI(p.Metrics),
			GoodQuery:     stringOrPrior(p.Sli.GoodQuery, prior.GoodQuery),
			TotalQuery:    stringOrPrior(p.Sli.TotalQuery, prior.TotalQuery),
			Resolution:    durationValue(p.Resolution, prior.Resolution),
			Target:        types.Float64Value(p.Target),
			WindowKind:    types.StringPointerValue(p.WindowKind),
			RollingWindow: durationValue(p.RollingWindow, prior.RollingWindow),
		}
		if len(p.Grouping) > 0 || (!prior.Grouping.IsNull() && !prior.Grouping.IsUnknown()) {
			v, d := types.ListValueFrom(ctx, types.StringType, p.Grouping)
			diags.Append(d...)
			m.Grouping = v
		} else {
			m.Grouping = types.ListNull(types.StringType)
		}

		rules := make([]sloRuleModel, len(p.Rules))
		for i, r := range p.Rules {
			var pr sloRuleModel
			if i < len(priorRules) {
				pr = priorRules[i]
			}
			rules[i] = sloRuleModel{
				Name:          types.StringValue(r.Name),
				Priority:      types.StringValue(r.Priority),
				Type:          types.StringValue(r.Type),
				LongWindow:    optionalDuration(r.Config.LongWindow, pr.LongWindow),
				ShortWindow:   optionalDuration(r.Config.ShortWindow, pr.ShortWindow),
				Factor:        float64Value(r.Config.Factor),
				ConsumedBound: float64Value(r.Config.ConsumedBound),
				MinTotal:      types.Float64Value(r.Config.MinTotal),
			}
			if r.Config.MinTotal == 0 && pr.MinTotal.IsNull() {
				rules[i].MinTotal = types.Float64Null()
			}
		}
		v, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: sloRuleAttrTypes}, rules)
		diags.Append(d...)
		m.Rules = v

		return paramsToObject(ctx, sloKind, &m, &diags), diags
	}
}
