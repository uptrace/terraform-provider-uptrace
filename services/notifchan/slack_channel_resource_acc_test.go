package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccSlackChannelConfigWebhook(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_slack_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  auth_method = "webhook"
  webhook_url = %q
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccSlackChannel_webhook(t *testing.T) {
	const (
		orgName     = "acc-slack-webhook-org"
		projectName = "acc-slack-webhook-project"
		webhookURL  = "https://hooks.slack.com/services/T00/B00/XXXX"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_slack_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccSlackChannelConfigWebhook(orgName, projectName, "acc-slack-webhook", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_slack_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "name", "acc-slack-webhook"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "match_all", "true"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "priorities.#", "1"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "auth_method", "webhook"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "webhook_url", webhookURL),
					resource.TestCheckResourceAttrSet("uptrace_slack_channel.test", "status"),
				),
			},
			{
				Config:   testAccSlackChannelConfigWebhook(orgName, projectName, "acc-slack-webhook", webhookURL),
				PlanOnly: true,
			},
			{
				Config: testAccSlackChannelConfigWebhook(orgName, projectName, "acc-slack-webhook-renamed", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "name", "acc-slack-webhook-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_slack_channel.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Sensitive fields are redacted by the API on read.
				ImportStateVerifyIgnore: []string{"webhook_url"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_slack_channel.test"),
			},
		},
	})
}

func testAccSlackChannelConfigToken(orgName, projectName, channelName, token, slackChannel string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_slack_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  auth_method = "token"
  token       = %q
  channel     = %q
}
`, orgName, projectName, channelName, token, slackChannel)
}

func TestAccSlackChannel_token(t *testing.T) {
	const (
		orgName      = "acc-slack-token-org"
		projectName  = "acc-slack-token-project"
		token        = "xoxb-test-token-value"
		slackChannel = "#alerts"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_slack_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccSlackChannelConfigToken(orgName, projectName, "acc-slack-token", token, slackChannel),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "auth_method", "token"),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "token", token),
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "channel", slackChannel),
				),
			},
			{
				Config:   testAccSlackChannelConfigToken(orgName, projectName, "acc-slack-token", token, slackChannel),
				PlanOnly: true,
			},
			{
				Config: testAccSlackChannelConfigToken(orgName, projectName, "acc-slack-token-renamed", token, slackChannel),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_slack_channel.test", "name", "acc-slack-token-renamed"),
				),
			},
			{
				ResourceName:            "uptrace_slack_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_slack_channel.test"),
			},
		},
	})
}
