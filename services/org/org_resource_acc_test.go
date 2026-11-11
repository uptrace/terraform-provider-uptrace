package org_test

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

func testAccOrgConfig(name string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}
`, name)
}

func testAccOrgConfigWithBudget(name string, budget float64) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name   = %q
  budget = %v
}
`, name, budget)
}

func testAccCheckOrgDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_org" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org ID %q: %w", rs.Primary.ID, err)
			}
			_, err = c.API.GetOrg(context.Background(), api.GetOrgRequest{OrgID: orgID})
			if err == nil {
				return fmt.Errorf("org %s still exists after destroy", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking org %s after destroy: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func TestAccOrg_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgConfig("acc-test-org"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org.test", "name", "acc-test-org"),
					resource.TestCheckResourceAttrSet("uptrace_org.test", "budget"),
				),
			},
			{
				Config: testAccOrgConfig("acc-test-org-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org.test", "name", "acc-test-org-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_org.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccOrg_disappearsOutOfBand(t *testing.T) {
	config := testAccOrgConfig("acc-disappear-org")
	var orgID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org.test", "id"),
					captureResourceID("uptrace_org.test", &orgID),
				),
			},
			{
				// Delete the org out-of-band, then re-apply the same config.
				// Terraform's Read should detect the 404, remove the resource
				// from state, and the plan should recreate it.
				PreConfig: func() {
					deleteOrgOutOfBand(t, orgID)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org.test", "name", "acc-disappear-org"),
				),
			},
		},
	})
}

func captureResourceID(resourceAddr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("resource %s not found", resourceAddr)
		}
		*dest = rs.Primary.ID
		return nil
	}
}

func deleteOrgOutOfBand(t *testing.T, orgID string) {
	t.Helper()
	if orgID == "" {
		t.Fatal("org ID was not captured before out-of-band delete")
	}

	id, err := strconv.ParseUint(orgID, 10, 64)
	if err != nil {
		t.Fatalf("invalid org ID %q: %v", orgID, err)
	}

	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteOrg(context.Background(), api.DeleteOrgRequest{OrgID: id})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete org %d out-of-band: %v", id, err)
	}
}

func TestAccOrg_withBudget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgConfigWithBudget("acc-budget-org", 250),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org.test", "name", "acc-budget-org"),
					resource.TestCheckResourceAttr("uptrace_org.test", "budget", "250"),
				),
			},
			{
				Config: testAccOrgConfigWithBudget("acc-budget-org", 500),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org.test", "budget", "500"),
				),
			},
			{
				ResourceName:      "uptrace_org.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccOrg_removeBudgetKeepsCurrentBudget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgConfigWithBudget("acc-budget-removed-org", 500),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org.test", "budget", "500"),
				),
			},
			{
				Config: testAccOrgConfig("acc-budget-removed-org"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org.test", "budget", "500"),
				),
			},
		},
	})
}
