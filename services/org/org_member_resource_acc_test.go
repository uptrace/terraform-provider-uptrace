package org_test

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

func testAccOrgMemberConfig(orgName, email, role, teamIDs string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = "acc-om-team"
}

resource "uptrace_org_member" "test" {
  org_id   = uptrace_org.test.id
  email    = %q
  role     = %q
  team_ids = %s
}
`, orgName, email, role, teamIDs)
}

func testAccCheckOrgMemberDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_org_member" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id: %w", err)
			}
			email := rs.Primary.Attributes["email"]
			_, err = c.API.GetOrgMember(context.Background(), &generated.GetOrgMemberRequestOptions{
				PathParams: &generated.GetOrgMemberPath{OrgID: orgID},
				Query:      &generated.GetOrgMemberQuery{Email: email},
			})
			if err == nil {
				return fmt.Errorf("org_member %s still exists in org %d after destroy", email, orgID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("get org_member after destroy: %w", err)
			}
		}
		return nil
	}
}

// TestAccOrgMember_basic invites an address with no account, changes the role
// and teams of the pending membership, and imports it.
func TestAccOrgMember_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-org-member")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgMemberDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgMemberConfig("acc-om-org", email, "member", "[uptrace_team.test.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org_member.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "email", email),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "role", "member"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "status", "pending"),
					resource.TestCheckNoResourceAttr("uptrace_org_member.test", "org_user_id"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "team_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(
						"uptrace_org_member.test", "team_ids.*", "uptrace_team.test", "id"),
				),
			},
			{
				Config: testAccOrgMemberConfig("acc-om-org", email, "admin", "[]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_member.test", "role", "admin"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "status", "pending"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "team_ids.#", "0"),
				),
			},
			{
				Config:   testAccOrgMemberConfig("acc-om-org", email, "admin", "[]"),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_org_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_org_member.test"]
					if !ok {
						return "", fmt.Errorf("resource uptrace_org_member.test not found")
					}
					return rs.Primary.Attributes["org_id"] + ":" + rs.Primary.Attributes["email"], nil
				},
			},
		},
	})
}
