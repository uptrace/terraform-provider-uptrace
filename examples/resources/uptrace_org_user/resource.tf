# uptrace_org_user cannot create a membership: use uptrace_org_member, which
# invites a person by email. It still manages a membership that is already in
# the state, or one imported by <org_id>:<org_user_id>.
import {
  to = uptrace_org_user.alice
  id = "123:456"
}

resource "uptrace_org_user" "alice" {
  org_id  = "123"
  user_id = "789"
  role    = "admin"
}
