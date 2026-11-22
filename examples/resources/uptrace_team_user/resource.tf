resource "uptrace_org" "example" {
  name = "My Organization"
}

resource "uptrace_team" "example" {
  org_id = uptrace_org.example.id
  name   = "platform"
}

# The org_user_id of a member. A member that this configuration invites with
# uptrace_org_member has one once the invitation is accepted:
# uptrace_org_member.<name>.org_user_id.
data "uptrace_org_member" "alice" {
  org_id = uptrace_org.example.id
  email  = "alice@example.com"
}

resource "uptrace_team_user" "example" {
  org_id      = uptrace_org.example.id
  team_id     = uptrace_team.example.id
  org_user_id = data.uptrace_org_member.alice.org_user_id
}
