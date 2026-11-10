package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccPushoverChannelConfig(orgName, projectName, channelName, token, userKey string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_pushover_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  token      = %q
  user_key   = %q
  priority   = 1
}
`, orgName, projectName, channelName, token, userKey)
}

func TestAccPushoverChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-po-org"
		projectName = "acc-po-project"
		token       = "po-app-token"
		userKey     = "po-user-key"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_pushover_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccPushoverChannelConfig(orgName, projectName, "acc-po-basic", token, userKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_pushover_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_pushover_channel.test", "name", "acc-po-basic"),
					resource.TestCheckResourceAttr("uptrace_pushover_channel.test", "token", token),
					resource.TestCheckResourceAttr("uptrace_pushover_channel.test", "user_key", userKey),
					resource.TestCheckResourceAttr("uptrace_pushover_channel.test", "priority", "1"),
					resource.TestCheckResourceAttrSet("uptrace_pushover_channel.test", "status"),
				),
			},
			{
				Config:   testAccPushoverChannelConfig(orgName, projectName, "acc-po-basic", token, userKey),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_pushover_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token", "user_key"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_pushover_channel.test"),
			},
		},
	})
}
