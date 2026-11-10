package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccMattermostChannelConfig(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_mattermost_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  webhook_url = %q
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccMattermostChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-mm-org"
		projectName = "acc-mm-project"
		webhookURL  = "https://mattermost.example.com/hooks/abcdef"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_mattermost_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccMattermostChannelConfig(orgName, projectName, "acc-mm-basic", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_mattermost_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_mattermost_channel.test", "name", "acc-mm-basic"),
					resource.TestCheckResourceAttr("uptrace_mattermost_channel.test", "webhook_url", webhookURL),
					resource.TestCheckResourceAttrSet("uptrace_mattermost_channel.test", "status"),
				),
			},
			{
				Config:   testAccMattermostChannelConfig(orgName, projectName, "acc-mm-basic", webhookURL),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_mattermost_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"webhook_url"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_mattermost_channel.test"),
			},
		},
	})
}
