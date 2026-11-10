terraform {
  required_providers {
    uptrace = {
      source = "uptrace/uptrace"
    }
  }
}

# The "Users and teams" setup guide of the Uptrace UI shows the part of this
# file after the "Users and teams" line. Teams are a Premium feature.

resource "uptrace_org" "main" {
  name = "My Organization"
}

resource "uptrace_project" "api" {
  org_id = uptrace_org.main.id
  name   = "api"
}

# --- Users and teams ---

resource "uptrace_user" "alice" {
  email = "alice@example.com"
}

resource "uptrace_org_user" "alice" {
  org_id  = uptrace_org.main.id
  user_id = uptrace_user.alice.id
  role    = "admin"
}

resource "uptrace_team" "platform" {
  org_id = uptrace_org.main.id
  name   = "Platform"
}

resource "uptrace_team_user" "alice_platform" {
  org_id      = uptrace_org.main.id
  team_id     = uptrace_team.platform.id
  org_user_id = uptrace_org_user.alice.id
}

resource "uptrace_team_project" "platform_api" {
  org_id     = uptrace_org.main.id
  team_id    = uptrace_team.platform.id
  project_id = uptrace_project.api.id
}
