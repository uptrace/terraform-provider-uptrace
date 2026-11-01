# Alert three days before a filesystem is projected to be 96% full.
resource "uptrace_forecast_monitor" "example" {
  project_id = uptrace_project.example.id
  name       = "Filesystem fills up on {{ .Attrs.host_name }}"

  params = {
    metrics   = [{ name = "system_filesystem_usage", alias = "$fs" }]
    query     = "sum($fs{state=\"used\"}) / sum($fs) as util | group by host_name | group by device"
    column    = { name = "util", unit = "utilization" }
    max_value = 0.96
    horizon   = "3d"
  }
}
