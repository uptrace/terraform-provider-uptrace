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

resource "uptrace_team" "platform" {
  org_id = uptrace_org.main.id
  name   = "Platform"
}

# Invites alice@example.com to the organization and the team. She joins when
# she accepts the emailed invitation.
resource "uptrace_org_member" "alice" {
  org_id   = uptrace_org.main.id
  email    = "alice@example.com"
  role     = "admin"
  team_ids = [uptrace_team.platform.id]
}

resource "uptrace_team_project" "platform_api" {
  org_id     = uptrace_org.main.id
  team_id    = uptrace_team.platform.id
  project_id = uptrace_project.api.id
}
