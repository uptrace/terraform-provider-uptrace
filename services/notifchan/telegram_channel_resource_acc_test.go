package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccTelegramChannelConfig(orgName, projectName, channelName string, chatID int64) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_telegram_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  chat_id    = %d
}
`, orgName, projectName, channelName, chatID)
}

func TestAccTelegramChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-tg-org"
		projectName = "acc-tg-project"
		chatID      = int64(-100123456)
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_telegram_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccTelegramChannelConfig(orgName, projectName, "acc-tg-basic", chatID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_telegram_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_telegram_channel.test", "name", "acc-tg-basic"),
					resource.TestCheckResourceAttr("uptrace_telegram_channel.test", "chat_id", "-100123456"),
					resource.TestCheckResourceAttrSet("uptrace_telegram_channel.test", "status"),
				),
			},
			{
				Config:   testAccTelegramChannelConfig(orgName, projectName, "acc-tg-basic", chatID),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_telegram_channel.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importStateIDFunc("uptrace_telegram_channel.test"),
			},
		},
	})
}
