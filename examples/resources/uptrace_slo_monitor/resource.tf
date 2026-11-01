# A 99.9% success SLO over 30 days, alerting on a fast burn.
resource "uptrace_slo_monitor" "example" {
  project_id = uptrace_project.example.id
  name       = "Span success SLO"

  params = {
    metrics        = [{ name = "uptrace_tracing_spans", alias = "$spans" }]
    good_query     = "histogram_count($spans{_status_code=\"ok\"})"
    total_query    = "histogram_count($spans)"
    resolution     = "1m"
    target         = 0.999
    window_kind    = "rolling"
    rolling_window = "30d"

    rules = [{
      name         = "fast burn"
      priority     = "high"
      type         = "burn_rate"
      long_window  = "1h"
      short_window = "5m"
      factor       = 14.4
    }]
  }
}
