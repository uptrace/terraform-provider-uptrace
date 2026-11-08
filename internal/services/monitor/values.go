package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	str2duration "github.com/xhit/go-str2duration/v2"
)

// durationMillis parses a duration attribute into the whole milliseconds the
// API takes. A null or unknown value yields nil, so the key is omitted.
func durationMillis(v types.String) (*int64, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	d, err := str2duration.ParseDuration(v.ValueString())
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", v.ValueString(), err)
	}
	ms := d.Milliseconds()
	return &ms, nil
}

// durationValue maps milliseconds from the API to a duration attribute. It
// keeps the prior spelling when that names the same duration ("60s" stays
// "60s", not "1m"), so a refresh never shows a diff the server did not make.
func durationValue(ms int64, prior types.String) types.String {
	if p, err := durationMillis(prior); err == nil && p != nil && *p == ms {
		return prior
	}
	return types.StringValue(formatDuration(time.Duration(ms) * time.Millisecond))
}

// formatDuration writes d the way a person would: "1m" rather than "1m0s",
// and "3d" rather than "72h" for whole days.
func formatDuration(d time.Duration) string {
	const day = 24 * time.Hour
	if d == 0 {
		return "0s"
	}
	if d%day == 0 {
		return fmt.Sprintf("%dd", d/day)
	}
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func stringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func int64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func float64Ptr(v types.Float64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := v.ValueFloat64()
	return &f
}

func float64Value(p *float64) types.Float64 {
	if p == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*p)
}

// stringOrPrior keeps the prior value when it is set. The server normalizes
// some inputs (an MQL query gains "{}" after each metric alias), so the
// configured spelling stays the source of truth; an import, which has no
// prior value, takes the server's.
func stringOrPrior(api string, prior types.String) types.String {
	if !prior.IsNull() && !prior.IsUnknown() {
		return prior
	}
	return types.StringValue(api)
}

// durationValidator accepts a non-negative duration string.
type durationValidator struct{}

func (durationValidator) Description(context.Context) string {
	return `must be a duration string, such as "90s", "5m" or "30d"`
}

func (v durationValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (durationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	d, err := str2duration.ParseDuration(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid duration",
			fmt.Sprintf("%q is not a duration string: %s", req.ConfigValue.ValueString(), err))
		return
	}
	if d < 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid duration",
			fmt.Sprintf("%q must not be negative", req.ConfigValue.ValueString()))
	}
}

// durationBetween accepts a duration string from min to max inclusive. It
// holds a knob to the bounds the server's Validate holds it to, so a value the
// server would refuse, or rewrite to its default as it does a zero, fails the
// plan instead of the apply.
type durationBetween struct {
	min, max time.Duration
}

func (v durationBetween) Description(context.Context) string {
	return fmt.Sprintf("must be a duration from %s to %s", formatDuration(v.min), formatDuration(v.max))
}

func (v durationBetween) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v durationBetween) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	d, err := str2duration.ParseDuration(req.ConfigValue.ValueString())
	if err != nil {
		return // durationValidator reports it
	}
	if d < v.min || d > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, "duration out of range",
			fmt.Sprintf("%q %s; omit it for the server default", req.ConfigValue.ValueString(), v.Description(context.Background())))
	}
}
