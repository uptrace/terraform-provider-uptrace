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
