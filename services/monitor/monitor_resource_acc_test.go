package monitor_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/testutil"
)

func testAccProjectConfig(org, project string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}
`, org, project)
}

func importIDFunc(addr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", addr)
		}
		return rs.Primary.Attributes["project_id"] + ":" + rs.Primary.ID, nil
	}
}

// testAccMonitorSteps returns create, re-plan, update, re-plan and import
// steps for one monitor whose config the two HCL bodies give.
func testAccMonitorSteps(addr, base, create, update string, createChecks, updateChecks []resource.TestCheckFunc) []resource.TestStep {
	return []resource.TestStep{
		{Config: base + create, Check: resource.ComposeAggregateTestCheckFunc(createChecks...)},
		{Config: base + create, PlanOnly: true},
		{Config: base + update, Check: resource.ComposeAggregateTestCheckFunc(updateChecks...)},
		{Config: base + update, PlanOnly: true},
		{
			ResourceName:      addr,
			ImportState:       true,
			ImportStateVerify: true,
			ImportStateIdFunc: importIDFunc(addr),
			// The server normalizes an MQL query (each alias gains "{}"). The
			// resource keeps the configured spelling, which an import lacks.
			ImportStateVerifyIgnore: []string{"status", "params.query", "params.good_query", "params.total_query"},
		},
	}
}

func TestAccThresholdMonitor_channelsAndRecovery(t *testing.T) {
	const addr = "uptrace_threshold_monitor.test"
	base := testAccProjectConfig("acc-threshold-org", "acc-threshold-project") + `
resource "uptrace_telegram_channel" "test" {
  project_id = uptrace_project.test.id
  name       = "acc-threshold-tg"
  priorities = ["high"]
  chat_id    = -100123456
}
`
	create := `
resource "uptrace_threshold_monitor" "test" {
  project_id  = uptrace_project.test.id
  name        = "acc process stopped reporting"
  description = "{{ .Attrs.service_name }} sent no runtime metrics"
  channel_ids = [uptrace_telegram_channel.test.id]

  repeat_interval = {
    strategy = "fixed"
    interval = "1h"
  }

  params = {
    metrics         = [{ name = "go_goroutine_count", alias = "$goroutines" }]
    query           = "sum($goroutines) as goroutines | group by service_name"
    column          = { name = "goroutines" }
    resolution      = "1m"
    num_eval_points = 5
    absent_points   = "alert"
    min_value       = 1
  }
}
`
	update := `
resource "uptrace_threshold_monitor" "test" {
  project_id  = uptrace_project.test.id
  name        = "acc process stopped reporting"
  channel_ids = []

  params = {
    metrics       = [{ name = "go_goroutine_count", alias = "$goroutines" }]
    query         = "sum($goroutines) as goroutines | group by service_name"
    absent_points = "zero"
    max_value     = 10000
    recovery      = { max_value = 8000 }
  }
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_threshold_monitor"),
		Steps: testAccMonitorSteps(addr, base, create, update,
			[]resource.TestCheckFunc{
				resource.TestCheckResourceAttrSet(addr, "id"),
				resource.TestCheckResourceAttr(addr, "channel_ids.#", "1"),
				resource.TestCheckResourceAttrPair(addr, "channel_ids.0", "uptrace_telegram_channel.test", "id"),
				resource.TestCheckResourceAttr(addr, "repeat_interval.strategy", "fixed"),
				resource.TestCheckResourceAttr(addr, "repeat_interval.interval", "1h"),
				resource.TestCheckResourceAttr(addr, "params.resolution", "1m"),
				resource.TestCheckResourceAttr(addr, "params.min_value", "1"),
				resource.TestCheckNoResourceAttr(addr, "params.max_value"),
			},
			[]resource.TestCheckFunc{
				// channel_ids = [] must detach the channel server-side.
				resource.TestCheckResourceAttr(addr, "channel_ids.#", "0"),
				resource.TestCheckNoResourceAttr(addr, "description"),
				resource.TestCheckResourceAttr(addr, "params.absent_points", "zero"),
				resource.TestCheckResourceAttr(addr, "params.max_value", "10000"),
				resource.TestCheckResourceAttr(addr, "params.recovery.max_value", "8000"),
				resource.TestCheckNoResourceAttr(addr, "params.recovery.min_value"),
				// The repeat interval stays as stored when the config drops it.
				resource.TestCheckResourceAttr(addr, "repeat_interval.strategy", "fixed"),
			},
		),
	})
}

func TestAccErrorMonitor_basic(t *testing.T) {
	const addr = "uptrace_error_monitor.test"
	base := testAccProjectConfig("acc-error-org", "acc-error-project")
	monitor := func(query string) string {
		return fmt.Sprintf(`
resource "uptrace_error_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc error logs"

  params = {
    metrics = [{ name = "uptrace_tracing_logs", alias = "$logs" }]
    query   = %q
  }
}
`, query)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_error_monitor"),
		Steps: testAccMonitorSteps(addr, base,
			monitor(`sum($logs) | group by _group_id | where _system in ("log:error", "log:fatal")`),
			monitor(`sum($logs) | group by _group_id | where _system = "log:fatal"`),
			[]resource.TestCheckFunc{resource.TestCheckResourceAttrSet(addr, "id")},
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.query",
				`sum($logs) | group by _group_id | where _system = "log:fatal"`)},
		),
	})
}

func TestAccAnomalyMonitor_basic(t *testing.T) {
	const addr = "uptrace_anomaly_monitor.test"
	base := testAccProjectConfig("acc-anomaly-org", "acc-anomaly-project")
	monitor := func(sensitivity string) string {
		return fmt.Sprintf(`
resource "uptrace_anomaly_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc requests anomaly"

  params = {
    metrics     = [{ name = "uptrace_tracing_spans", alias = "$spans" }]
    query       = "histogram_count($spans) | group by host_name"
    sensitivity = %q
    direction   = "both"
  }
}
`, sensitivity)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_anomaly_monitor"),
		Steps: testAccMonitorSteps(addr, base, monitor("medium"), monitor("high"),
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.sensitivity", "medium")},
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.sensitivity", "high")},
		),
	})
}

func TestAccOutlierMonitor_basic(t *testing.T) {
	const addr = "uptrace_outlier_monitor.test"
	base := testAccProjectConfig("acc-outlier-org", "acc-outlier-project")
	monitor := func(minPeers int) string {
		return fmt.Sprintf(`
resource "uptrace_outlier_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc cpu outlier"

  params = {
    metrics         = [{ name = "system_cpu_load_average_1m", alias = "$load" }]
    query           = "avg($load) | group by host_name"
    min_peers       = %d
    keep_firing_for = "10m"
  }
}
`, minPeers)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_outlier_monitor"),
		Steps: testAccMonitorSteps(addr, base, monitor(3), monitor(4),
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.keep_firing_for", "10m")},
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.min_peers", "4")},
		),
	})
}

func TestAccForecastMonitor_basic(t *testing.T) {
	const addr = "uptrace_forecast_monitor.test"
	base := testAccProjectConfig("acc-forecast-org", "acc-forecast-project")
	monitor := func(maxValue string) string {
		return fmt.Sprintf(`
resource "uptrace_forecast_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc filesystem forecast"

  params = {
    metrics   = [{ name = "system_filesystem_usage", alias = "$fs" }]
    query     = "sum($fs{state=\"used\"}) / sum($fs) as util | group by host_name"
    column    = { name = "util", unit = "utilization" }
    max_value = %s
    horizon   = "3d"
  }
}
`, maxValue)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_forecast_monitor"),
		Steps: testAccMonitorSteps(addr, base, monitor("0.96"), monitor("0.9"),
			[]resource.TestCheckFunc{
				resource.TestCheckResourceAttr(addr, "params.horizon", "3d"),
				resource.TestCheckResourceAttr(addr, "params.max_value", "0.96"),
			},
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.max_value", "0.9")},
		),
	})
}

func TestAccSLOMonitor_basic(t *testing.T) {
	const addr = "uptrace_slo_monitor.test"
	base := testAccProjectConfig("acc-slo-org", "acc-slo-project")
	monitor := func(target string) string {
		return fmt.Sprintf(`
resource "uptrace_slo_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc spans SLO"

  params = {
    metrics        = [{ name = "uptrace_tracing_spans", alias = "$spans" }]
    good_query     = "histogram_count($spans{_status_code=\"ok\"})"
    total_query    = "histogram_count($spans)"
    resolution     = "1m"
    target         = %s
    window_kind    = "rolling"
    rolling_window = "30d"

    rules = [
      {
        name         = "fast burn"
        priority     = "high"
        type         = "burn_rate"
        long_window  = "1h"
        short_window = "5m"
        factor       = 14.4
      },
      {
        name           = "half spent"
        priority       = "medium"
        type           = "budget_consumed"
        consumed_bound = 0.5
        min_total      = 10
      },
    ]
  }
}
`, target)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_slo_monitor"),
		Steps: testAccMonitorSteps(addr, base, monitor("0.999"), monitor("0.99"),
			[]resource.TestCheckFunc{
				resource.TestCheckResourceAttr(addr, "params.rules.#", "2"),
				resource.TestCheckResourceAttr(addr, "params.rules.0.long_window", "1h"),
				resource.TestCheckResourceAttr(addr, "params.rolling_window", "30d"),
			},
			[]resource.TestCheckFunc{resource.TestCheckResourceAttr(addr, "params.target", "0.99")},
		),
	})
}

// TestAccThresholdMonitor_deletedOutOfBand checks that a monitor deleted
// outside Terraform is planned for creation again.
func TestAccThresholdMonitor_deletedOutOfBand(t *testing.T) {
	const addr = "uptrace_threshold_monitor.test"
	config := testAccProjectConfig("acc-threshold-oob-org", "acc-threshold-oob-project") + `
resource "uptrace_threshold_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = "acc out of band"

  params = {
    metrics   = [{ name = "go_goroutine_count", alias = "$g" }]
    query     = "sum($g)"
    max_value = 100000
  }
}
`
	var projectID, monitorID string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_threshold_monitor"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr(addr, "project_id", &projectID),
					testutil.CaptureAttr(addr, "id", &monitorID),
				),
			},
			{
				PreConfig:          func() { deleteMonitorOutOfBand(t, projectID, monitorID) },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
