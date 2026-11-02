resource "uptrace_org" "example" {
  name = "My Organization"
}

resource "uptrace_project" "example" {
  org_id = uptrace_org.example.id
  name   = "api"
}

# Email every member of the project.
resource "uptrace_email_channel" "everyone" {
  project_id = uptrace_project.example.id
  name       = "alerts-email"
  priorities = ["high"]
  everyone   = true
}

resource "uptrace_team" "oncall" {
  org_id = uptrace_org.example.id
  name   = "oncall"
}

resource "uptrace_team_project" "oncall" {
  org_id     = uptrace_org.example.id
  team_id    = uptrace_team.oncall.id
  project_id = uptrace_project.example.id
}

# Email the members of one team.
resource "uptrace_email_channel" "oncall" {
  project_id = uptrace_project.example.id
  name       = "oncall-email"
  priorities = ["high", "medium"]
  team_ids   = [uptrace_team.oncall.id]
}
