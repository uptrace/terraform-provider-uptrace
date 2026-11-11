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

func testAccOrgUserConfig(orgName, email, role string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_user" "test" {
  email = %q
}

resource "uptrace_org_user" "test" {
  org_id  = uptrace_org.test.id
  user_id = uptrace_user.test.id
  role    = %q
}
`, orgName, email, role)
}

func testAccCheckOrgUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_org_user" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id: %w", err)
			}
			orgUserID, err := strconv.ParseUint(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id: %w", err)
			}
			_, err = c.API.GetOrgUser(context.Background(), api.GetOrgUserRequest{OrgID: orgID, OrgUserID: orgUserID})
			if err == nil {
				return fmt.Errorf("org_user %d still exists in org %d after destroy", orgUserID, orgID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("get org_user after destroy: %w", err)
			}
		}
		return nil
	}
}

func TestAccOrgUser_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-org-user")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgUserConfig("acc-ou-org", email, "member"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "member"),
				),
			},
			{
				Config: testAccOrgUserConfig("acc-ou-org", email, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "admin"),
				),
			},
			{
				Config:   testAccOrgUserConfig("acc-ou-org", email, "admin"),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_org_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importOrgUserIDFunc("uptrace_org_user.test"),
			},
		},
	})
}

func importOrgUserIDFunc(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", resourceAddr)
		}
		return fmt.Sprintf("%s:%s",
			rs.Primary.Attributes["org_id"],
			rs.Primary.Attributes["id"],
		), nil
	}
}
