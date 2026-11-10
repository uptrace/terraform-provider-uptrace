package user_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/uptrace/terraform/testutil"
)

func testAccUserConfig(email string) string {
	return fmt.Sprintf(`
resource "uptrace_user" "test" {
  email = %q
}
`, email)
}

func TestAccUser_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-user")
	updatedEmail := testutil.AcceptanceTestUserEmail("acc-user-updated")
	var originalID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_user.test", "email", email),
					resource.TestCheckResourceAttrSet("uptrace_user.test", "id"),
					testutil.CaptureAttr("uptrace_user.test", "id", &originalID),
				),
			},
			{
				Config:   testAccUserConfig(email),
				PlanOnly: true,
			},
			{
				Config: testAccUserConfig(updatedEmail),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("uptrace_user.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_user.test", "email", updatedEmail),
					resource.TestCheckResourceAttrSet("uptrace_user.test", "id"),
					resource.TestCheckResourceAttrWith("uptrace_user.test", "id", func(id string) error {
						if id == originalID {
							return fmt.Errorf("expected email change to replace user ID %q", id)
						}
						return nil
					}),
				),
			},
			{
				Config:   testAccUserConfig(updatedEmail),
				PlanOnly: true,
			},
		},
	})
}
