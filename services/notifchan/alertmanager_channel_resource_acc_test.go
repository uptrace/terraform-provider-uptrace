package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/testutil"
)

func testAccAlertmanagerChannelConfigBasicAuth(orgName, projectName, channelName, url, username, password string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_alertmanager_channel" "test" {
  project_id  = uptrace_project.test.id
  name        = %q
  priorities  = ["high"]
  url         = %q
  auth_method = "basic_auth"
  username    = %q
  password    = %q
}
`, orgName, projectName, channelName, url, username, password)
}

func TestAccAlertmanagerChannel_basicAuth(t *testing.T) {
	const (
		orgName     = "acc-am-basic-org"
		projectName = "acc-am-basic-project"
		url         = "https://alertmanager.example.com/api/v2/alerts"
		username    = "probe"
		password    = "probe-secret"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_alertmanager_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccAlertmanagerChannelConfigBasicAuth(orgName, projectName, "acc-am-basic", url, username, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_alertmanager_channel.test", "id"),
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "name", "acc-am-basic"),
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "url", url),
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "auth_method", "basic_auth"),
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "username", username),
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "password", password),
					resource.TestCheckResourceAttrSet("uptrace_alertmanager_channel.test", "status"),
				),
			},
			{
				Config:   testAccAlertmanagerChannelConfigBasicAuth(orgName, projectName, "acc-am-basic", url, username, password),
				PlanOnly: true,
			},
			{
				Config: testAccAlertmanagerChannelConfigBasicAuth(orgName, projectName, "acc-am-basic-renamed", url, username, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_alertmanager_channel.test", "name", "acc-am-basic-renamed"),
				),
			},
			{
				ResourceName:            "uptrace_alertmanager_channel.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
				ImportStateIdFunc:       importStateIDFunc("uptrace_alertmanager_channel.test"),
			},
		},
	})
}
