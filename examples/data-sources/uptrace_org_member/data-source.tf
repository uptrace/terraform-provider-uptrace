resource "uptrace_org" "main" {
  name = "Main"
}

# The membership of the person at alice@example.com: active, with an
# org_user_id, or pending while an invitation waits.
data "uptrace_org_member" "alice" {
  org_id = uptrace_org.main.id
  email  = "alice@example.com"
}

output "alice_org_user_id" {
  value = data.uptrace_org_member.alice.org_user_id
}
