# Alert when a host's request rate leaves its learned normal range.
resource "uptrace_anomaly_monitor" "example" {
  project_id = uptrace_project.example.id
  name       = "Request rate anomaly on {{ .Attrs.host_name }}"

  params = {
    metrics     = [{ name = "uptrace_tracing_spans", alias = "$spans" }]
    query       = "histogram_count($spans{_type=\"httpserver\"}) | group by host_name"
    sensitivity = "medium"
    direction   = "both"
  }
}
