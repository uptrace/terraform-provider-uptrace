package team_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/testutil"
)

func testAccTeamConfig(orgName, teamName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}
`, orgName, teamName)
}

func testAccTeamConfigWithPermLevel(orgName, teamName, permLevel string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_team" "test" {
  org_id     = uptrace_org.test.id
  name       = %q
  perm_level = %q
}
`, orgName, teamName, permLevel)
}

func testAccCheckTeamDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_team" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id %q: %w", rs.Primary.Attributes["org_id"], err)
			}
			teamID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid team ID %q: %w", rs.Primary.ID, err)
			}
			_, err = c.API.GetTeam(context.Background(), api.GetTeamRequest{OrgID: orgID, TeamID: teamID})
			if err == nil {
				return fmt.Errorf("team %d still exists after destroy", teamID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("checking team %d after destroy: %w", teamID, err)
			}
		}
		return nil
	}
}

func TestAccTeam_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfig("acc-team-org", "acc-team-basic"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_team.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_team.test", "org_id"),
					resource.TestCheckResourceAttr("uptrace_team.test", "name", "acc-team-basic"),
					// perm_level is Computed — the backend fills in the
					// default when the config omits it.
					resource.TestCheckResourceAttrSet("uptrace_team.test", "perm_level"),
				),
			},
			{
				Config:   testAccTeamConfig("acc-team-org", "acc-team-basic"),
				PlanOnly: true,
			},
			{
				Config: testAccTeamConfig("acc-team-org", "acc-team-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_team.test", "name", "acc-team-renamed"),
				),
			},
			{
				ResourceName:      "uptrace_team.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importStateIDFunc("uptrace_team.test"),
			},
		},
	})
}

func TestAccTeam_withPermLevel(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigWithPermLevel("acc-team-perm-org", "acc-team-perm", "view"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_team.test", "perm_level", "view"),
				),
			},
			{
				Config: testAccTeamConfigWithPermLevel("acc-team-perm-org", "acc-team-perm", "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_team.test", "perm_level", "admin"),
				),
			},
			{
				// Removing perm_level from config preserves the
				// current backend value (perm_level is Optional +
				// Computed; UseStateForUnknown carries it forward).
				Config: testAccTeamConfig("acc-team-perm-org", "acc-team-perm"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_team.test", "perm_level", "admin"),
				),
			},
		},
	})
}

func TestAccTeam_disappearsOutOfBand(t *testing.T) {
	config := testAccTeamConfig("acc-team-disappear-org", "acc-team-disappear")
	var orgID, teamID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr("uptrace_team.test", "org_id", &orgID),
					testutil.CaptureAttr("uptrace_team.test", "id", &teamID),
				),
			},
			{
				PreConfig: func() { deleteTeamOutOfBand(t, orgID, teamID) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_team.test", "name", "acc-team-disappear"),
				),
			},
		},
	})
}

func deleteTeamOutOfBand(t *testing.T, orgIDStr, teamIDStr string) {
	t.Helper()
	if orgIDStr == "" || teamIDStr == "" {
		t.Fatal("org_id or team_id was not captured before out-of-band delete")
	}
	orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid org_id %q: %v", orgIDStr, err)
	}
	teamID, err := strconv.ParseUint(teamIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid team_id %q: %v", teamIDStr, err)
	}
	c := testutil.TestAccClient(t)
	_, err = c.API.DeleteTeam(context.Background(), api.DeleteTeamRequest{OrgID: orgID, TeamID: teamID})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("delete team %d out-of-band: %v", teamID, err)
	}
}

func importStateIDFunc(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", resourceAddr)
		}
		return fmt.Sprintf("%s:%s", rs.Primary.Attributes["org_id"], rs.Primary.ID), nil
	}
}
