# Alert when one host's load strays from its peers'.
resource "uptrace_outlier_monitor" "example" {
  project_id = uptrace_project.example.id
  name       = "CPU load outlier: {{ .Attrs.host_name }}"

  params = {
    metrics         = [{ name = "system_cpu_load_average_1m", alias = "$load" }]
    query           = "avg($load) | group by host_name"
    min_peers       = 3
    keep_firing_for = "10m"
  }
}
