package org_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

// testAccOrgMemberDataConfig reads two memberships of a new org by email: the
// owner's, which is active, and one that uptrace_org_member invites, which is
// pending until the invitation is accepted.
func testAccOrgMemberDataConfig(orgName, ownerEmail, inviteeEmail string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

data "uptrace_org_member" "owner" {
  org_id = uptrace_org.test.id
  email  = %q
}

resource "uptrace_org_member" "invitee" {
  org_id = uptrace_org.test.id
  email  = %q
  role   = "viewer"
}

data "uptrace_org_member" "invitee" {
  org_id = uptrace_org.test.id
  email  = uptrace_org_member.invitee.email
}
`, orgName, ownerEmail, inviteeEmail)
}

func TestAccOrgMemberDataSource_basic(t *testing.T) {
	ownerEmail := testutil.AccountEmail(t)
	inviteeEmail := testutil.AcceptanceTestUserEmail("acc-om-data")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgMemberDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgMemberDataConfig("acc-om-data-org", ownerEmail, inviteeEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptrace_org_member.owner", "status", "active"),
					resource.TestCheckResourceAttr("data.uptrace_org_member.owner", "role", "owner"),
					resource.TestCheckResourceAttrSet("data.uptrace_org_member.owner", "org_user_id"),
					resource.TestCheckResourceAttr("data.uptrace_org_member.invitee", "status", "pending"),
					resource.TestCheckResourceAttr("data.uptrace_org_member.invitee", "role", "viewer"),
					resource.TestCheckNoResourceAttr("data.uptrace_org_member.invitee", "org_user_id"),
				),
			},
			{
				Config:   testAccOrgMemberDataConfig("acc-om-data-org", ownerEmail, inviteeEmail),
				PlanOnly: true,
			},
		},
	})
}
