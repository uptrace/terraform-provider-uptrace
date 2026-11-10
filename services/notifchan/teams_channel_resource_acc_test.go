package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccTeamsChannelConfig(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_teams_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  webhook_url = %q
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccTeamsChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-teams-org"
		projectName = "acc-teams-project"
		webhookURL  = "https://outlook.office.com/webhook/AAAA/IncomingWebhook/BBBB/CCCC"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_teams_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccTeamsChannelConfig(orgName, projectName, "acc-teams-basic", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_teams_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_teams_channel.test", "name", "acc-teams-basic"),
					resource.TestCheckResourceAttr("uptrace_teams_channel.test", "webhook_url", webhookURL),
					resource.TestCheckResourceAttrSet("uptrace_teams_channel.test", "status"),
				),
			},
			{
				Config:   testAccTeamsChannelConfig(orgName, projectName, "acc-teams-basic", webhookURL),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_teams_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"webhook_url"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_teams_channel.test"),
			},
		},
	})
}
