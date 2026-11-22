package org_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/testutil"
)

// testAccOrgUserConfig holds an org, the membership of its owner (the
// provider's own account) read by email, and, when withOrgUser is set, an
// uptrace_org_user of that membership.
func testAccOrgUserConfig(orgName, ownerEmail string, ownerUserID uint64, withOrgUser bool) string {
	config := fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

data "uptrace_org_member" "owner" {
  org_id = uptrace_org.test.id
  email  = %q
}
`, orgName, ownerEmail)
	if withOrgUser {
		config += fmt.Sprintf(`
resource "uptrace_org_user" "test" {
  org_id  = uptrace_org.test.id
  user_id = %q
  role    = "owner"
}
`, strconv.FormatUint(ownerUserID, 10))
	}
	return config
}

// TestAccOrgUser_createRefused pins that a new uptrace_org_user fails at plan
// time: the server no longer attaches a user to an org by user ID.
func TestAccOrgUser_createRefused(t *testing.T) {
	email, userID := testutil.AccountEmail(t), testutil.AccountUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      testAccOrgUserConfig("acc-ou-refused-org", email, userID, true),
				ExpectError: regexp.MustCompile(`can no longer create a membership`),
			},
		},
	})
}

// TestAccOrgUser_import pins the path an existing membership keeps: an import
// by <org_id>:<org_user_id> reads it. The test imports the owner's membership
// and does not persist it, so the destroy at the end removes the org and never
// the owner's last membership.
func TestAccOrgUser_import(t *testing.T) {
	email, userID := testutil.AccountEmail(t), testutil.AccountUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgUserConfig("acc-ou-import-org", email, userID, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.uptrace_org_member.owner", "org_user_id"),
				),
			},
			{
				Config:            testAccOrgUserConfig("acc-ou-import-org", email, userID, true),
				ResourceName:      "uptrace_org_user.test",
				ImportState:       true,
				ImportStateIdFunc: importOwnerOrgUserIDFunc,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("imported %d instances, want 1", len(states))
					}
					got := states[0].Attributes
					if want := strconv.FormatUint(userID, 10); got["user_id"] != want {
						return fmt.Errorf("user_id = %q, want %q", got["user_id"], want)
					}
					if got["role"] != "owner" {
						return fmt.Errorf("role = %q, want owner", got["role"])
					}
					return nil
				},
			},
		},
	})
}

// importOwnerOrgUserIDFunc builds "<org_id>:<org_user_id>" from the owner's
// membership that the data source read.
func importOwnerOrgUserIDFunc(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources["data.uptrace_org_member.owner"]
	if !ok {
		return "", fmt.Errorf("data.uptrace_org_member.owner not found")
	}
	return fmt.Sprintf("%s:%s",
		rs.Primary.Attributes["org_id"],
		rs.Primary.Attributes["org_user_id"],
	), nil
}
