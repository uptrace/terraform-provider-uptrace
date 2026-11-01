# Alert on each new group of error and fatal logs.
resource "uptrace_error_monitor" "example" {
  project_id = uptrace_project.example.id
  name       = "Notify on all errors"

  params = {
    metrics = [{ name = "uptrace_tracing_logs", alias = "$logs" }]
    query   = "sum($logs) | group by _group_id | where _system in (\"log:error\", \"log:fatal\")"
  }
}
