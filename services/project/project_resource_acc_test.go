package project_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/testutil"
)

func testAccProjectConfig(orgName, projectName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}
`, orgName, projectName)
}

func testAccProjectConfigWithRetention(orgName, projectName, spanRetention, spanTimeRange string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id          = uptrace_org.test.id
  name            = %q
  span_retention  = %q
  span_time_range = %q
}
`, orgName, projectName, spanRetention, spanTimeRange)
}

func testAccCheckProjectDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_project" {
				continue
			}
			projectID, err := strconv.ParseUint(rs.Primary.ID, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid project ID %q: %w", rs.Primary.ID, err)
			}
			_, err = c.API.GetProject(context.Background(), &generated.GetProjectRequestOptions{
				PathParams: &generated.GetProjectPath{ProjectID: uint32(projectID)},
			})
			if err == nil {
				return fmt.Errorf("project %s still exists after destroy", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking project %s after destroy: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func TestAccProject_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig("acc-test-project-org", "acc-test-project"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_project.test", "org_id"),
					resource.TestCheckResourceAttr("uptrace_project.test", "name", "acc-test-project"),
				),
			},
			{
				Config: testAccProjectConfig("acc-test-project-org", "acc-test-project-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project.test", "name", "acc-test-project-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_project.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_project.test"]
					if !ok {
						return "", fmt.Errorf("uptrace_project.test not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["org_id"], rs.Primary.ID), nil
				},
			},
		},
	})
}

func TestAccProject_retentionRoundTrip(t *testing.T) {
	const (
		spanRetention = "672h" // server minimum
		spanTimeRange = "6h"
		bumped        = "1000h"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfigWithRetention("acc-retention-org", "acc-retention-project", spanRetention, spanTimeRange),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project.test", "span_retention", spanRetention),
					resource.TestCheckResourceAttr("uptrace_project.test", "span_time_range", spanTimeRange),
				),
			},
			{
				// No change — should be a clean no-op plan despite CE's
				// Read-side retention override.
				Config:   testAccProjectConfigWithRetention("acc-retention-org", "acc-retention-project", spanRetention, spanTimeRange),
				PlanOnly: true,
			},
			{
				// User bumps retention — should apply cleanly.
				Config: testAccProjectConfigWithRetention("acc-retention-org", "acc-retention-project", bumped, spanTimeRange),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project.test", "span_retention", bumped),
				),
			},
		},
	})
}

func TestAccProject_disappearsOutOfBand(t *testing.T) {
	config := testAccProjectConfig("acc-disappear-project-org", "acc-disappear-project")
	var projectID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project.test", "id"),
					captureProjectID("uptrace_project.test", &projectID),
				),
			},
			{
				PreConfig: func() {
					deleteProjectOutOfBand(t, projectID)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project.test", "id"),
					resource.TestCheckResourceAttr("uptrace_project.test", "name", "acc-disappear-project"),
				),
			},
		},
	})
}

func captureProjectID(resourceAddr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found", resourceAddr)
		}
		*dest = rs.Primary.ID
		return nil
	}
}

func deleteProjectOutOfBand(t *testing.T, projectID string) {
	t.Helper()
	if projectID == "" {
		t.Fatal("project ID was not captured before out-of-band delete")
	}

	id, err := strconv.ParseUint(projectID, 10, 32)
	if err != nil {
		t.Fatalf("invalid project ID %q: %v", projectID, err)
	}

	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteProject(context.Background(), &generated.DeleteProjectRequestOptions{
		PathParams: &generated.DeleteProjectPath{ProjectID: uint32(id)},
	})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete project %d out-of-band: %v", id, err)
	}
}
