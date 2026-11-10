package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccIncidentioChannelConfig(orgName, projectName, channelName, url, apiKey string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_incidentio_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  url        = %q
  api_key    = %q
}
`, orgName, projectName, channelName, url, apiKey)
}

func TestAccIncidentioChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-io-org"
		projectName = "acc-io-project"
		url         = "https://api.incident.io/v2/alert_events/http/ACC"
		apiKey      = "io-api-key-test"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_incidentio_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentioChannelConfig(orgName, projectName, "acc-io-basic", url, apiKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_incidentio_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_incidentio_channel.test", "name", "acc-io-basic"),
					resource.TestCheckResourceAttr("uptrace_incidentio_channel.test", "url", url),
					resource.TestCheckResourceAttr("uptrace_incidentio_channel.test", "api_key", apiKey),
					resource.TestCheckResourceAttrSet("uptrace_incidentio_channel.test", "status"),
				),
			},
			{
				Config:   testAccIncidentioChannelConfig(orgName, projectName, "acc-io-basic", url, apiKey),
				PlanOnly: true,
			},
			{
				ResourceName:            "uptrace_incidentio_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_incidentio_channel.test"),
			},
		},
	})
}
