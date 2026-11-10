package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccPagerdutyChannelConfig(orgName, projectName, channelName, routingKey string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_pagerduty_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  routing_key = %q
  severity    = "error"
}
`, orgName, projectName, channelName, routingKey)
}

func TestAccPagerdutyChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-pagerduty-org"
		projectName = "acc-pagerduty-project"
		routingKey  = "R01ABCDEFGHIJKLMN"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_pagerduty_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccPagerdutyChannelConfig(orgName, projectName, "acc-pagerduty", routingKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_pagerduty_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_pagerduty_channel.test", "name", "acc-pagerduty"),
					resource.TestCheckResourceAttr("uptrace_pagerduty_channel.test", "routing_key", routingKey),
					resource.TestCheckResourceAttr("uptrace_pagerduty_channel.test", "severity", "error"),
					resource.TestCheckResourceAttrSet("uptrace_pagerduty_channel.test", "status"),
				),
			},
			{
				Config:   testAccPagerdutyChannelConfig(orgName, projectName, "acc-pagerduty", routingKey),
				PlanOnly: true,
			},
			{
				Config: testAccPagerdutyChannelConfig(orgName, projectName, "acc-pagerduty-renamed", routingKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_pagerduty_channel.test", "name", "acc-pagerduty-renamed"),
				),
			},
			{
				ResourceName:            "uptrace_pagerduty_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"routing_key"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_pagerduty_channel.test"),
			},
		},
	})
}
