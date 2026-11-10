package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccOpsgenieChannelConfig(orgName, projectName, channelName, apiKey, priority string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_opsgenie_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  api_key    = %q
  priority   = %q
}
`, orgName, projectName, channelName, apiKey, priority)
}

func TestAccOpsgenieChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-opsgenie-org"
		projectName = "acc-opsgenie-project"
		apiKey      = "og-api-key-test"
		priority    = "P2"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_opsgenie_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccOpsgenieChannelConfig(orgName, projectName, "acc-opsgenie-basic", apiKey, priority),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_opsgenie_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_opsgenie_channel.test", "name", "acc-opsgenie-basic"),
					resource.TestCheckResourceAttr("uptrace_opsgenie_channel.test", "api_key", apiKey),
					resource.TestCheckResourceAttr("uptrace_opsgenie_channel.test", "priority", priority),
					resource.TestCheckResourceAttrSet("uptrace_opsgenie_channel.test", "status"),
				),
			},
			{
				Config:   testAccOpsgenieChannelConfig(orgName, projectName, "acc-opsgenie-basic", apiKey, priority),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_opsgenie_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_opsgenie_channel.test"),
			},
		},
	})
}
