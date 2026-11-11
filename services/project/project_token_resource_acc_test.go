package project_test

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

func testAccProjectTokenConfig(orgName, projectName, tokenName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_project_token" "test" {
  project_id = uptrace_project.test.id
  name       = %q
}
`, orgName, projectName, tokenName)
}

func testAccCheckProjectTokenDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_project_token" {
				continue
			}
			tokenID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid token ID %q: %w", rs.Primary.ID, err)
			}
			projectID, err := strconv.ParseUint(rs.Primary.Attributes["project_id"], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid project ID %q: %w", rs.Primary.Attributes["project_id"], err)
			}
			_, err = c.API.GetProjectToken(context.Background(), api.GetProjectTokenRequest{ProjectID: uint32(projectID), TokenID: tokenID})
			if err == nil {
				return fmt.Errorf("project token %s still exists after destroy", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking project token %s after destroy: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func TestAccProjectToken_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectTokenDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectTokenConfig("acc-token-org", "acc-token-project", "initial"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "project_id"),
					resource.TestCheckResourceAttr("uptrace_project_token.test", "name", "initial"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "token"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "dsn"),
				),
			},
			{
				Config: testAccProjectTokenConfig("acc-token-org", "acc-token-project", "renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project_token.test", "name", "renamed"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "token"),
				),
			},
			{
				ResourceName:      "uptrace_project_token.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_project_token.test"]
					if !ok {
						return "", fmt.Errorf("uptrace_project_token.test not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["project_id"], rs.Primary.ID), nil
				},
			},
		},
	})
}

func TestAccProjectToken_withName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectTokenDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectTokenConfig("acc-token-name-org", "acc-token-name-project", "ci-ingest"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project_token.test", "name", "ci-ingest"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "token"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "dsn"),
				),
			},
			{
				Config:   testAccProjectTokenConfig("acc-token-name-org", "acc-token-name-project", "ci-ingest"),
				PlanOnly: true,
			},
			{
				Config: testAccProjectTokenConfig("acc-token-name-org", "acc-token-name-project", "renamed-ingest"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_project_token.test", "name", "renamed-ingest"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "token"),
				),
			},
			{
				Config:   testAccProjectTokenConfig("acc-token-name-org", "acc-token-name-project", "renamed-ingest"),
				PlanOnly: true,
			},
		},
	})
}

func TestAccProjectToken_disappearsOutOfBand(t *testing.T) {
	config := testAccProjectTokenConfig("acc-disappear-token-org", "acc-disappear-token-project", "disappear-test")
	var tokenID, projectID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectTokenDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "id"),
					testutil.CaptureAttr("uptrace_project_token.test", "id", &tokenID),
					testutil.CaptureAttr("uptrace_project_token.test", "project_id", &projectID),
				),
			},
			{
				PreConfig: func() {
					deleteProjectTokenOutOfBand(t, projectID, tokenID)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_project_token.test", "token"),
				),
			},
		},
	})
}

func deleteProjectTokenOutOfBand(t *testing.T, projectIDStr, tokenIDStr string) {
	t.Helper()
	if projectIDStr == "" || tokenIDStr == "" {
		t.Fatal("project ID or token ID was not captured before out-of-band delete")
	}

	projectID, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		t.Fatalf("invalid project ID %q: %v", projectIDStr, err)
	}
	tokenID, err := strconv.ParseUint(tokenIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid token ID %q: %v", tokenIDStr, err)
	}

	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteProjectToken(context.Background(), api.DeleteProjectTokenRequest{ProjectID: uint32(projectID), TokenID: tokenID})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete project token %d out-of-band: %v", tokenID, err)
	}
}
