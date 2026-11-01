resource "uptrace_telegram_channel" "ops" {
  project_id = uptrace_project.example.id
  name       = "Ops"
  priorities = ["info", "low", "medium", "high"]
  chat_id    = -1001234567890
}

# Alert when a service stops sending runtime metrics: a crash, a crash loop,
# or a deploy that never starts.
resource "uptrace_threshold_monitor" "example" {
  project_id  = uptrace_project.example.id
  name        = "{{ .Attrs.service_name }} stopped reporting"
  channel_ids = [uptrace_telegram_channel.ops.id]

  params = {
    metrics         = [{ name = "go_goroutine_count", alias = "$goroutines" }]
    query           = "sum($goroutines) as goroutines | group by service_name"
    resolution      = "1m"
    num_eval_points = 5
    absent_points   = "alert"
    min_value       = 1
  }
}

# Alert above a bound, and recover only below a lower one.
resource "uptrace_threshold_monitor" "queue" {
  project_id = uptrace_project.example.id
  name       = "Queue depth"

  params = {
    metrics   = [{ name = "taskq_consumers_queue_length", alias = "$length" }]
    query     = "max($length) as length | group by queue"
    max_value = 10000
    recovery  = { max_value = 8000 }
  }
}
