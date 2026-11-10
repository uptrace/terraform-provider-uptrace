package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccServicenowChannelConfig(orgName, projectName, channelName, url, username, password string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_servicenow_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  url        = %q
  username   = %q
  password   = %q
  category   = "software"
  impact     = "2"
  urgency    = "1"
}
`, orgName, projectName, channelName, url, username, password)
}

func TestAccServicenowChannel_basic(t *testing.T) {
	const (
		orgName     = "acc-servicenow-org"
		projectName = "acc-servicenow-project"
		url         = "https://instance.service-now.com"
		username    = "alertuser"
		password    = "alertsecret"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_servicenow_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccServicenowChannelConfig(orgName, projectName, "acc-servicenow", url, username, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_servicenow_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "name", "acc-servicenow"),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "url", url),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "username", username),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "password", password),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "category", "software"),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "impact", "2"),
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "urgency", "1"),
					resource.TestCheckResourceAttrSet("uptrace_servicenow_channel.test", "status"),
				),
			},
			{
				Config:   testAccServicenowChannelConfig(orgName, projectName, "acc-servicenow", url, username, password),
				PlanOnly: true,
			},
			{
				Config: testAccServicenowChannelConfig(orgName, projectName, "acc-servicenow-renamed", url, username, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_servicenow_channel.test", "name", "acc-servicenow-renamed"),
				),
			},
			{
				ResourceName:            "uptrace_servicenow_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"url", "password"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_servicenow_channel.test"),
			},
		},
	})
}
