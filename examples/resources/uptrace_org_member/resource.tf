resource "uptrace_org" "main" {
  name = "Main"
}

resource "uptrace_team" "platform" {
  org_id = uptrace_org.main.id
  name   = "Platform"
}

# Invites alice@example.com. The membership is pending until she accepts the
# emailed invitation, and active after.
resource "uptrace_org_member" "alice" {
  org_id   = uptrace_org.main.id
  email    = "alice@example.com"
  role     = "admin"
  team_ids = [uptrace_team.platform.id]
}

resource "uptrace_project" "billing" {
  org_id = uptrace_org.main.id
  name   = "Billing"
}

# Gives bob@example.com view on every project but billing, which the override
# at none keeps them out of. Overrides set outside Terraform stay as they are
# when project_overrides is omitted.
resource "uptrace_org_member" "bob" {
  org_id = uptrace_org.main.id
  email  = "bob@example.com"
  role   = "viewer"

  project_overrides = {
    (uptrace_project.billing.id) = "none"
  }
}
