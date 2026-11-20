package org_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/api"
	"github.com/uptrace/terraform/client"
	"github.com/uptrace/terraform/services/org"
	"github.com/uptrace/terraform/testutil"
)

func testAccOrgMemberConfig(orgName, email, role, teamIDs string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_team" "test" {
  org_id = uptrace_org.test.id
  name   = "acc-om-team"
}

resource "uptrace_org_member" "test" {
  org_id   = uptrace_org.test.id
  email    = %q
  role     = %q
  team_ids = %s
}
`, orgName, email, role, teamIDs)
}

func testAccCheckOrgMemberDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_org_member" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id: %w", err)
			}
			email := rs.Primary.Attributes["email"]
			_, err = c.API.GetOrgMember(context.Background(), api.GetOrgMemberRequest{OrgID: orgID, Email: email})
			if err == nil {
				return fmt.Errorf("org_member %s still exists in org %d after destroy", email, orgID)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("get org_member after destroy: %w", err)
			}
		}
		return nil
	}
}

// TestAccOrgMember_basic invites an address with no account, changes the role
// and teams of the pending membership, and imports it.
func TestAccOrgMember_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-org-member")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgMemberDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgMemberConfig("acc-om-org", email, "member", "[uptrace_team.test.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org_member.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "email", email),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "role", "member"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "status", "pending"),
					resource.TestCheckNoResourceAttr("uptrace_org_member.test", "org_user_id"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "team_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(
						"uptrace_org_member.test", "team_ids.*", "uptrace_team.test", "id"),
				),
			},
			{
				Config: testAccOrgMemberConfig("acc-om-org", email, "admin", "[]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_member.test", "role", "admin"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "status", "pending"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "team_ids.#", "0"),
				),
			},
			{
				Config:   testAccOrgMemberConfig("acc-om-org", email, "admin", "[]"),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_org_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				// An import reads the overrides the server holds, and this
				// configuration leaves them unmanaged, so its state holds none
				// (ADR-0466).
				ImportStateVerifyIgnore: []string{"project_overrides"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_org_member.test"]
					if !ok {
						return "", fmt.Errorf("resource uptrace_org_member.test not found")
					}
					return rs.Primary.Attributes["org_id"] + ":" + rs.Primary.Attributes["email"], nil
				},
			},
		},
	})
}

func testAccOrgMemberOverridesConfig(email, role, overrides string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = "acc-om-overrides-org"
}

resource "uptrace_project" "denied" {
  org_id = uptrace_org.test.id
  name   = "acc-om-denied"
}

resource "uptrace_project" "raised" {
  org_id = uptrace_org.test.id
  name   = "acc-om-raised"
}

resource "uptrace_org_member" "test" {
  org_id            = uptrace_org.test.id
  email             = %q
  role              = %q
  project_overrides = %s
}
`, email, role, overrides)
}

// testAccCheckOverride checks the level of the override on the project that
// the uptrace_project named project manages.
func testAccCheckOverride(project, level string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["uptrace_project."+project]
		if !ok {
			return fmt.Errorf("uptrace_project.%s not found", project)
		}
		return resource.TestCheckResourceAttr(
			"uptrace_org_member.test", "project_overrides."+rs.Primary.ID, level)(s)
	}
}

// testAccCheckOverrideLevelsMatchServer sends each role with each level to the
// server and checks that it refuses exactly the pairs that the provider
// refuses at plan time (org.IsValidOverride). The provider reads the role table
// from the contract it was built from, so this catches a server whose
// contract states another table. For each refused pair it also checks that
// the provider's sentence (org.OverrideRefusal) is the server's field detail
// without the project, which the attribute path names.
func testAccCheckOverrideLevelsMatchServer(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["uptrace_project.raised"]
		if !ok {
			return errors.New("uptrace_project.raised not found")
		}
		orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
		if err != nil {
			return err
		}
		projectID, err := strconv.ParseUint(rs.Primary.ID, 10, 32)
		if err != nil {
			return err
		}

		c := testutil.TestAccClient(t)
		roles := []api.UserRole{
			api.UserRoleOwner, api.UserRoleAdmin, api.UserRoleMember,
			api.UserRoleViewer, api.UserRoleBillingManager, api.UserRoleCollaborator,
		}
		levels := []api.PermLevel{api.PermLevelNone, api.PermLevelView, api.PermLevelEdit, api.PermLevelAdmin}
		for _, role := range roles {
			for _, level := range levels {
				email := testutil.AcceptanceTestUserEmail(fmt.Sprintf("acc-om-%s-%s", role, level))
				overrides := []api.OrgMemberProjectOverride{{ProjectID: uint32(projectID), PermLevel: level}}
				_, err := c.API.SetOrgMember(context.Background(), api.SetOrgMemberRequest{OrgID: orgID},
					api.OrgMemberSetRequest{Email: email, Role: role, TeamIDs: []uint64{}, ProjectOverrides: &overrides})
				if err == nil {
					_, err := c.API.RemoveOrgMember(context.Background(),
						api.RemoveOrgMemberRequest{OrgID: orgID, Email: email})
					if err != nil {
						return fmt.Errorf("cancel the invitation of %s: %w", email, err)
					}
				}
				if refused, want := err != nil, !org.IsValidOverride(role, level); refused != want {
					return fmt.Errorf("role %s, level %s: the server refused it: %t, the provider refuses it: %t (%v)",
						role, level, refused, want, err)
				}
				if err != nil {
					if err := matchOverrideRefusal(err, role, level, projectID); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
}

// matchOverrideRefusal checks that the server refused the override with the
// sentence the provider gives at plan time: the server names the project
// ("must hold L or none on project N, ..."), and the provider leaves it to
// the attribute path ("must be L or none, ...").
func matchOverrideRefusal(err error, role api.UserRole, level api.PermLevel, projectID uint64) error {
	apiErr, ok := errors.AsType[*api.Error](err)
	if !ok {
		return fmt.Errorf("role %s, level %s: the refusal is not an API error: %w", role, level, err)
	}
	var detail string
	for _, fieldErr := range apiErr.Problem.Errors {
		if fieldErr.Field == "projectOverrides" {
			detail = fieldErr.Detail
		}
	}
	server := strings.Replace(detail, fmt.Sprintf(" on project %d", projectID), "", 1)
	server = strings.Replace(server, "must hold ", "must be ", 1)
	if provider := org.OverrideRefusal(role, level); server != provider {
		return fmt.Errorf("role %s, level %s: the server refuses with %q, the provider with %q",
			role, level, detail, provider)
	}
	return nil
}

// TestAccOrgMember_projectOverrides holds the overrides of a pending
// membership, reads them back, imports them, and refuses at plan time a level
// below the role. Dropping the attribute leaves the overrides on the server.
func TestAccOrgMember_projectOverrides(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-org-member-overrides")
	overrides := `{
    (uptrace_project.denied.id) = "none"
    (uptrace_project.raised.id) = "edit"
  }`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgMemberDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgMemberOverridesConfig(email, "member", overrides),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_member.test", "status", "pending"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "project_overrides.%", "2"),
					testAccCheckOverride("denied", "none"),
					testAccCheckOverride("raised", "edit"),
					testAccCheckOverrideLevelsMatchServer(t),
				),
			},
			{
				Config:   testAccOrgMemberOverridesConfig(email, "member", overrides),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_org_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_org_member.test"]
					if !ok {
						return "", fmt.Errorf("resource uptrace_org_member.test not found")
					}
					return rs.Primary.Attributes["org_id"] + ":" + rs.Primary.Attributes["email"], nil
				},
			},
			{
				// An admin gets admin on every project, so edit would lower it.
				Config:      testAccOrgMemberOverridesConfig(email, "admin", overrides),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`project override below the role`),
			},
			{
				// A promotion that keeps only the deny is taken in one apply.
				Config: testAccOrgMemberOverridesConfig(email, "admin", `{
    (uptrace_project.denied.id) = "none"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_member.test", "role", "admin"),
					resource.TestCheckResourceAttr("uptrace_org_member.test", "project_overrides.%", "1"),
					testAccCheckOverride("denied", "none"),
				),
			},
			{
				Config: testAccOrgMemberOverridesConfig(email, "admin", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("uptrace_org_member.test", "project_overrides.%"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["uptrace_org_member.test"]
						orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
						if err != nil {
							return err
						}
						out, err := testutil.TestAccClient(t).API.GetOrgMember(context.Background(),
							api.GetOrgMemberRequest{OrgID: orgID, Email: email})
						if err != nil {
							return err
						}
						if len(out.Member.ProjectOverrides) != 1 {
							return fmt.Errorf("want the deny kept on the server, got %+v", out.Member.ProjectOverrides)
						}
						return nil
					},
				),
			},
		},
	})
}
