package monitor_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/testutil"
)

// testAccCheckMonitorDestroy verifies that every resource of the given Terraform
// type has been deleted server-side. Shared by the error-monitor and
// metric-monitor acceptance tests.
func testAccCheckMonitorDestroy(t *testing.T, resourceType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			monitorID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid monitor ID %q: %w", rs.Primary.ID, err)
			}
			projectID, err := strconv.ParseUint(rs.Primary.Attributes["project_id"], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid project ID %q: %w", rs.Primary.Attributes["project_id"], err)
			}
			_, err = c.API.GetMonitor(context.Background(), api.GetMonitorRequest{ProjectID: uint32(projectID), MonitorID: monitorID})
			if err == nil {
				return fmt.Errorf("monitor %s still exists after destroy", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking monitor %s after destroy: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func deleteMonitorOutOfBand(t *testing.T, projectIDStr, monitorIDStr string) {
	t.Helper()
	if projectIDStr == "" || monitorIDStr == "" {
		t.Fatal("project ID or monitor ID was not captured before out-of-band delete")
	}
	projectID, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		t.Fatalf("invalid project ID %q: %v", projectIDStr, err)
	}
	monitorID, err := strconv.ParseUint(monitorIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid monitor ID %q: %v", monitorIDStr, err)
	}

	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteMonitor(context.Background(), api.DeleteMonitorRequest{ProjectID: uint32(projectID), MonitorID: monitorID})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete monitor %d out-of-band: %v", monitorID, err)
	}
}
