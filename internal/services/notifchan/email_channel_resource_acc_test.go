package notifchan_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/internal/testutil"
)

const testAccEmailChannelBase = `
resource "uptrace_org" "test" {
  name = "acc-email-org"
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = "acc-email-project"
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = "acc-email-team"
}

resource "uptrace_team_project" "test" {
  org_id     = uptrace_org.test.id
  team_id    = uptrace_team.test.id
  project_id = uptrace_project.test.id
}
`

func testAccEmailChannelEveryoneConfig(name string) string {
	return testAccEmailChannelBase + fmt.Sprintf(`
resource "uptrace_email_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high"]
  everyone   = true
}
`, name)
}

func testAccEmailChannelTeamConfig(name string) string {
	return testAccEmailChannelBase + fmt.Sprintf(`
resource "uptrace_email_channel" "test" {
  project_id = uptrace_project.test.id
  name       = %q
  priorities = ["high", "medium"]
  team_ids   = [uptrace_team.test.id]

  depends_on = [uptrace_team_project.test]
}
`, name)
}

func TestAccEmailChannel_basic(t *testing.T) {
	const addr = "uptrace_email_channel.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelDestroy(t, "uptrace_email_channel"),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailChannelEveryoneConfig("acc-email-everyone"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "name", "acc-email-everyone"),
					resource.TestCheckResourceAttr(addr, "everyone", "true"),
					resource.TestCheckResourceAttr(addr, "team_ids.#", "0"),
					resource.TestCheckResourceAttr(addr, "user_ids.#", "0"),
					resource.TestCheckResourceAttrSet(addr, "status"),
				),
			},
			{
				Config:   testAccEmailChannelEveryoneConfig("acc-email-everyone"),
				PlanOnly: true,
			},
			{
				// Switch the audience in place: from everyone to one team.
				Config: testAccEmailChannelTeamConfig("acc-email-team"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "name", "acc-email-team"),
					resource.TestCheckResourceAttr(addr, "everyone", "false"),
					resource.TestCheckResourceAttr(addr, "team_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(addr, "team_ids.*", "uptrace_team.test", "id"),
					resource.TestCheckResourceAttr(addr, "priorities.#", "2"),
				),
			},
			{
				Config:   testAccEmailChannelTeamConfig("acc-email-team"),
				PlanOnly: true,
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importStateIDFunc(addr),
			},
		},
	})
}
