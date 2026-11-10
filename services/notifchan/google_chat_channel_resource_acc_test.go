package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccGoogleChatChannelConfig(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_google_chat_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  webhook_url = %q
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccGoogleChatChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-gc-org"
		projectName = "acc-gc-project"
		webhookURL  = "https://chat.googleapis.com/v1/spaces/AAAA/messages?key=K&token=T"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_google_chat_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccGoogleChatChannelConfig(orgName, projectName, "acc-gc-basic", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_google_chat_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_google_chat_channel.test", "name", "acc-gc-basic"),
					resource.TestCheckResourceAttr("uptrace_google_chat_channel.test", "webhook_url", webhookURL),
					resource.TestCheckResourceAttrSet("uptrace_google_chat_channel.test", "status"),
				),
			},
			{
				Config:   testAccGoogleChatChannelConfig(orgName, projectName, "acc-gc-basic", webhookURL),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_google_chat_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"webhook_url"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_google_chat_channel.test"),
			},
		},
	})
}
