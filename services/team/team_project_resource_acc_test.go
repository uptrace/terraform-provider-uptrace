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

func testAccTeamProjectConfig(orgName, projectName, teamName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_team_project" "test" {
  org_id     = uptrace_org.test.id
  team_id    = uptrace_team.test.id
  project_id = uptrace_project.test.id
}
`, orgName, projectName, teamName)
}

// testAccCheckTeamProjectDestroy verifies that after destroy, the project is
// no longer listed under the team. The team itself may also be gone, which
// is equally acceptable (counts as membership removed).
func testAccCheckTeamProjectDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_team_project" {
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
			projectID64, err := strconv.ParseUint(rs.Primary.Attributes["project_id"], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid project_id: %w", err)
			}
			projectID := uint32(projectID64)

			out, err := c.API.ListTeamProjects(context.Background(), &generated.ListTeamProjectsRequestOptions{
				PathParams: &generated.ListTeamProjectsPath{OrgID: orgID, TeamID: teamID},
			})
			if err != nil {
				if client.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("list team projects after destroy: %w", err)
			}
			for _, p := range out.Projects {
				if p.ID == projectID {
					return fmt.Errorf("project %d still associated with team %d after destroy", projectID, teamID)
				}
			}
		}
		return nil
	}
}

func TestAccTeamProject_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamProjectDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTeamProjectConfig("acc-tp-org", "acc-tp-project", "acc-tp-team"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_team_project.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_team_project.test", "org_id"),
					resource.TestCheckResourceAttrSet("uptrace_team_project.test", "team_id"),
					resource.TestCheckResourceAttrSet("uptrace_team_project.test", "project_id"),
					resource.TestCheckResourceAttrPair(
						"uptrace_team_project.test", "id",
						"uptrace_team_project.test", "project_id",
					),
				),
			},
			{
				Config:   testAccTeamProjectConfig("acc-tp-org", "acc-tp-project", "acc-tp-team"),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_team_project.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importTeamProjectIDFunc("uptrace_team_project.test"),
			},
		},
	})
}

func TestAccTeamProject_disappearsOutOfBand(t *testing.T) {
	config := testAccTeamProjectConfig("acc-tp-gone-org", "acc-tp-gone-project", "acc-tp-gone-team")
	var orgID, teamID, projectID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTeamProjectDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr("uptrace_team_project.test", "org_id", &orgID),
					testutil.CaptureAttr("uptrace_team_project.test", "team_id", &teamID),
					testutil.CaptureAttr("uptrace_team_project.test", "project_id", &projectID),
				),
			},
			{
				PreConfig: func() { removeTeamProjectOutOfBand(t, orgID, teamID, projectID) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_team_project.test", "id"),
				),
			},
		},
	})
}

func removeTeamProjectOutOfBand(t *testing.T, orgIDStr, teamIDStr, projectIDStr string) {
	t.Helper()
	if orgIDStr == "" || teamIDStr == "" || projectIDStr == "" {
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
	pid64, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		t.Fatalf("invalid project_id %q: %v", projectIDStr, err)
	}
	c := testutil.TestAccClient(t)
	_, err = c.API.RemoveTeamProject(context.Background(), &generated.RemoveTeamProjectRequestOptions{
		PathParams: &generated.RemoveTeamProjectPath{
			OrgID:     orgID,
			TeamID:    teamID,
			ProjectID: uint32(pid64),
		},
	})
	if err != nil && !client.IsNotFound(err) {
		t.Fatalf("remove team project out-of-band: %v", err)
	}
}

func importTeamProjectIDFunc(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", resourceAddr)
		}
		return fmt.Sprintf("%s:%s:%s",
			rs.Primary.Attributes["org_id"],
			rs.Primary.Attributes["team_id"],
			rs.Primary.Attributes["project_id"],
		), nil
	}
}
