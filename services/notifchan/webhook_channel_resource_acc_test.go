package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccWebhookChannelConfig(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_webhook_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  url        = %q
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccWebhookChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-webhook-channel-org"
		projectName = "acc-webhook-channel-project"
		webhookURL  = "https://example.com/alerts"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_webhook_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookChannelConfig(orgName, projectName, "acc-webhook-basic", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "project_id"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "name", "acc-webhook-basic"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "match_all", "true"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "priorities.#", "1"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "priorities.0", "high"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "url", webhookURL),
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "status"),
				),
			},
			{
				Config:   testAccWebhookChannelConfig(orgName, projectName, "acc-webhook-basic", webhookURL),
				PlanOnly: true,
			},
			{
				Config: testAccWebhookChannelConfig(orgName, projectName, "acc-webhook-basic-renamed", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "name", "acc-webhook-basic-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_webhook_channel.test",
				ImportState:       true,
				ImportStateVerify: true,
				// URL is Sensitive and the backend redacts it on read, so it can't
				// round-trip through import verification.
				ImportStateVerifyIgnore: []string{"url"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_webhook_channel.test"),
			},
		},
	})
}

func testAccWebhookChannelConfigWithPayload(orgName, projectName, channelName, webhookURL string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_webhook_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  url        = %q

  payload = jsonencode({
    source   = "uptrace"
    severity = "high"
  })
}
`, orgName, projectName, channelName, webhookURL)
}

func TestAccWebhookChannel_withPayload(t *testing.T) {
	const (
		orgName     = "acc-webhook-payload-org"
		projectName = "acc-webhook-payload-project"
		webhookURL  = "https://example.com/alerts"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_webhook_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookChannelConfigWithPayload(orgName, projectName, "acc-webhook-payload", webhookURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "payload"),
				),
			},
			{
				Config:   testAccWebhookChannelConfigWithPayload(orgName, projectName, "acc-webhook-payload", webhookURL),
				PlanOnly: true,
			},
		},
	})
}

func TestAccWebhookChannel_disappearsOutOfBand(t *testing.T) {
	const (
		orgName     = "acc-webhook-disappear-org"
		projectName = "acc-webhook-disappear-project"
		webhookURL  = "https://example.com/alerts"
	)
	config := testAccWebhookChannelConfig(orgName, projectName, "acc-webhook-disappear", webhookURL)
	var channelID, projectID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_webhook_channel"),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "id"),
					testutil.CaptureAttr("uptrace_webhook_channel.test", "id", &channelID),
					testutil.CaptureAttr("uptrace_webhook_channel.test", "project_id", &projectID),
				),
			},
			{
				PreConfig: func() {
					deleteChannelOutOfBand(t, projectID, channelID)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_webhook_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_webhook_channel.test", "name", "acc-webhook-disappear"),
				),
			},
		},
	})
}
