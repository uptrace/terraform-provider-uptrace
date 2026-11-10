package team_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/generated"
	"github.com/uptrace/terraform/testutil"
)

// testAccTeamUserConfig mirrors examples/resources/uptrace_team_user/resource.tf:
// org → team → user → org_user → team_user, all created inline so the test
// covers the full chain without relying on a pre-existing OrgUser ID.
func testAccTeamUserConfig(orgName, teamName, email string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_user" "test" {
  email = %q
}

resource "uptrace_org_user" "test" {
  org_id  = uptrace_org.test.id
  user_id = uptrace_user.test.id
  role    = "admin"
}

resource "uptrace_team_user" "test" {
  org_id      = uptrace_org.test.id
  team_id     = uptrace_team.test.id
  org_user_id = uptrace_org_user.test.id
}
`, orgName, teamName, email)
}

func testAccCheckTeamUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_team_user" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id: %w", err)
			}
			teamID, err := strconv.ParseUint(rs.Primary.Attributes["team_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid team_id: %w", err)
			}
			orgUserID, err := strconv.ParseUint(rs.Primary.Attributes["org_user_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_user_id: %w", err)
			}

			out, err := c.API.ListTeamUsers(context.Background(), &generated.ListTeamUsersRequestOptions{
				PathParams: &generated.ListTeamUsersPath{OrgID: orgID, TeamID: teamID},
			})
			if err != nil {
				if client.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("list team users after destroy: %w", err)
			}
			for _, u := range out.Users {
				if u.OrgUserID == orgUserID {
					return fmt.Errorf("org_user %d still member of team %d after destroy", orgUserID, teamID)
				}
			}
		}
		return nil
	}
}

func TestAccTeamUser_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-team-user")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTeamUserConfig("acc-tu-org", "acc-tu-team", email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_team_user.test", "id"),
					resource.TestCheckResourceAttrPair(
						"uptrace_team_user.test", "id",
						"uptrace_team_user.test", "org_user_id",
					),
				),
			},
			{
				Config:   testAccTeamUserConfig("acc-tu-org", "acc-tu-team", email),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_team_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importTeamUserIDFunc("uptrace_team_user.test"),
			},
		},
	})
}

func TestAccTeamUser_disappearsOutOfBand(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-team-user-gone")

	config := testAccTeamUserConfig("acc-tu-gone-org", "acc-tu-gone-team", email)
	var orgIDAttr, teamIDAttr, orgUserIDAttr string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr("uptrace_team_user.test", "org_id", &orgIDAttr),
					testutil.CaptureAttr("uptrace_team_user.test", "team_id", &teamIDAttr),
					testutil.CaptureAttr("uptrace_team_user.test", "org_user_id", &orgUserIDAttr),
				),
			},
			{
				PreConfig: func() { removeTeamUserOutOfBand(t, orgIDAttr, teamIDAttr, orgUserIDAttr) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_team_user.test", "id"),
				),
			},
		},
	})
}

func removeTeamUserOutOfBand(t *testing.T, orgIDStr, teamIDStr, orgUserIDStr string) {
	t.Helper()
	if orgIDStr == "" || teamIDStr == "" || orgUserIDStr == "" {
		t.Fatal("IDs not captured before out-of-band remove")
	}
	orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid org_id %q: %v", orgIDStr, err)
	}
	teamID, err := strconv.ParseUint(teamIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid team_id %q: %v", teamIDStr, err)
	}
	orgUserID, err := strconv.ParseUint(orgUserIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid org_user_id %q: %v", orgUserIDStr, err)
	}
	c := testutil.TestAccClient(t)
	_, err = c.API.RemoveTeamUser(context.Background(), &generated.RemoveTeamUserRequestOptions{
		PathParams: &generated.RemoveTeamUserPath{
			OrgID:     orgID,
			TeamID:    teamID,
			OrgUserID: orgUserID,
		},
	})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("remove team user out-of-band: %v", err)
	}
}

func importTeamUserIDFunc(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", resourceAddr)
		}
		return fmt.Sprintf("%s:%s:%s",
			rs.Primary.Attributes["org_id"],
			rs.Primary.Attributes["team_id"],
			rs.Primary.Attributes["org_user_id"],
		), nil
	}
}
