# Terraform Provider for Uptrace

Manage [Uptrace](https://uptrace.dev) organizations, projects, users, teams, monitors, and
notification channels with Terraform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0

## Quick start

A complete, ready-to-run example lives in [`examples/quickstart/`](examples/quickstart/). To try it:

```bash
cd examples/quickstart
terraform init
terraform plan -var="token=YOUR_API_TOKEN"
terraform apply -var="token=YOUR_API_TOKEN"
```

Or inline:

```hcl
terraform {
  required_providers {
    uptrace = {
      source = "uptrace/uptrace"
    }
  }
}

provider "uptrace" {
  endpoint = "https://api.uptrace.dev"
  token    = var.uptrace_token
}

resource "uptrace_org" "main" {
  name = "My Organization"
}

resource "uptrace_project" "api" {
  org_id         = uptrace_org.main.id
  name           = "api"
  span_retention = "30d"
}

resource "uptrace_project_token" "api" {
  project_id = uptrace_project.api.id
  name       = "default"
}
```

```bash
terraform init
terraform plan
terraform apply
```

## Provider configuration

| Argument   | Environment variable | Description              |
|------------|----------------------|--------------------------|
| `endpoint` | `UPTRACE_ENDPOINT`   | Uptrace API endpoint URL |
| `token`    | `UPTRACE_TOKEN`      | API authentication token |

Arguments take precedence over environment variables.

## Resources

### uptrace_org

Manages an organization.

| Field  | Type   | Required | Description                        |
|--------|--------|----------|------------------------------------|
| name   | string | yes      | Organization name. Updatable.      |
| budget | float  | no       | Budget limit. Updatable. Defaults to the server value when omitted; removing it later preserves the current value. |
| id     | string | computed |                                    |

### uptrace_project

Manages a project scoped under an organization.

| Field                  | Type   | Required | Description                                                      |
|------------------------|--------|----------|------------------------------------------------------------------|
| org_id                 | string | yes      | Organization ID. Forces replacement on change.                   |
| name                   | string | yes      | Project name. Updatable.                                         |
| group_by_env           | bool   | no       | Group spans by environment. Updatable.                           |
| group_funcs_by_service | bool   | no       | Group functions by service. Updatable.                           |
| semconv_version        | string | no       | One of `none`, `v1.25.0`, `v1.33.0`.                            |
| skip_monitor_status_emails   | bool | no   | Send no email when a monitor of the project changes status.     |
| skip_monitor_disabled_emails | bool | no   | Send no email when the server disables a monitor of the project. |
| span_time_range        | string | no       | Default query time range for spans (e.g. `"24h"`).              |
| log_time_range         | string | no       | Default query time range for logs.                               |
| event_time_range       | string | no       | Default query time range for events.                             |
| span_retention         | string | no       | Data retention for spans (e.g. `"30d"`, `"4w"`). Server minimum applies. |
| log_retention          | string | no       | Data retention for logs.                                         |
| event_retention        | string | no       | Data retention for events.                                       |
| metric_retention       | string | no       | Data retention for metrics.                                      |
| profile_retention      | string | no       | Data retention for profiles.                                     |
| id                     | string | computed |                                                                  |

Duration strings accept `ns`, `us`, `ms`, `s`, `m`, `h`, `d` (day), and `w` (week).

### uptrace_project_token

Manages an ingest token for a project.

| Field      | Type   | Required | Description                                               |
|------------|--------|----------|-----------------------------------------------------------|
| project_id | string | yes      | Project ID. Forces replacement on change.                 |
| name       | string | no       | Token name. Updatable. Removing it clears the name.       |
| id         | string | computed |                                                           |
| token      | string | computed | Sensitive. The generated ingest token.                    |
| dsn        | string | computed | Sensitive. Ingest URL with the token embedded.            |

Import: `terraform import uptrace_project_token.x <project_id>:<token_id>`

### uptrace_user

Invites a user to Uptrace. The backend sends an account-invitation email. If the email already
belongs to a confirmed user, the existing user is adopted (no duplicate is created).

| Field | Type   | Required | Description                                               |
|-------|--------|----------|-----------------------------------------------------------|
| email | string | yes      | Lowercase, trimmed. Forces replacement on change.         |
| id    | string | computed | Numeric user ID.                                          |

Delete removes the resource from state only -- the underlying user remains on the server.

### uptrace_org_user

Adds a user to an organization with a role. Pair with `uptrace_user` to manage the full
invite-to-membership lifecycle.

| Field   | Type   | Required | Description                                                                    |
|---------|--------|----------|--------------------------------------------------------------------------------|
| org_id  | string | yes      | Organization ID. Forces replacement on change.                                 |
| user_id | string | yes      | User ID (from `uptrace_user`). Forces replacement on change.                   |
| role    | string | yes      | One of `owner`, `admin`, `member`, `viewer`, `billing_manager`, `collaborator`. Updatable. |
| id      | string | computed | OrgUser ID. Pass to `uptrace_team_user.org_user_id`.                           |

Import: `terraform import uptrace_org_user.x <org_id>:<org_user_id>`

**Example -- invite a user and grant org membership:**

```hcl
resource "uptrace_user" "alice" {
  email = "alice@example.com"
}

resource "uptrace_org_user" "alice" {
  org_id  = uptrace_org.main.id
  user_id = uptrace_user.alice.id
  role    = "admin"
}
```

### uptrace_team

Manages a team within an organization. Teams are a **Premium** feature.

| Field      | Type   | Required | Description                                                    |
|------------|--------|----------|----------------------------------------------------------------|
| org_id     | string | yes      | Organization ID. Forces replacement on change.                 |
| name       | string | yes      | Team name (1-255 characters). Updatable.                       |
| perm_level | string | no       | One of `none`, `view`, `edit`, `admin`. Server fills the default when omitted. |
| id         | string | computed |                                                                |

Import: `terraform import uptrace_team.x <org_id>:<team_id>`

### uptrace_team_project

Grants a team access to a project. All fields force replacement (no in-place update).

| Field      | Type   | Required | Description                           |
|------------|--------|----------|---------------------------------------|
| org_id     | string | yes      | Organization ID.                      |
| team_id    | string | yes      | Team ID.                              |
| project_id | string | yes      | Project ID.                           |
| id         | string | computed | Equals `project_id`.                  |

Import: `terraform import uptrace_team_project.x <org_id>:<team_id>:<project_id>`

### uptrace_team_user

Adds an organization user to a team. All fields force replacement.

| Field       | Type   | Required | Description                                     |
|-------------|--------|----------|-------------------------------------------------|
| org_id      | string | yes      | Organization ID.                                |
| team_id     | string | yes      | Team ID.                                        |
| org_user_id | string | yes      | OrgUser ID (from `uptrace_org_user.id`).        |
| id          | string | computed | Equals `org_user_id`.                           |

Import: `terraform import uptrace_team_user.x <org_id>:<team_id>:<org_user_id>`

### Notification channels

Each notification-channel type is its own resource. All share a common set of fields plus
type-specific configuration.

**Shared fields:**

| Field       | Type         | Required | Description                                                           |
|-------------|--------------|----------|-----------------------------------------------------------------------|
| project_id  | string       | yes      | Project ID. Forces replacement on change.                             |
| name        | string       | yes      | Channel name. Updatable.                                              |
| priorities  | list(string) | yes      | Alert priorities to match: `info`, `low`, `medium`, `high`.          |
| match_all   | bool         | no       | Defaults to `true`. Set `false` to restrict to `monitor_ids`.        |
| monitor_ids | list(string) | no       | Required when `match_all = false`. Omit to leave the links alone; `[]` detaches every monitor. |
| condition   | string       | no       | Alert condition expression.                                           |
| id          | string       | computed |                                                                       |
| status      | string       | computed | One of `delivering`, `paused`, `disabled`, `draft`.                  |

**Type-specific fields:**

| Resource                         | Fields                                                                                                                           |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `uptrace_email_channel`          | `everyone` (bool), or `team_ids` / `user_ids` (sets of IDs). Set `everyone`, or at least one list; only project members get the email. |
| `uptrace_slack_channel`          | `auth_method` (`webhook` or `token`), `webhook_url`, `token`, `channel`. Required fields depend on `auth_method`; unused ones are sent empty. |
| `uptrace_google_chat_channel`    | `webhook_url`                                                                                                                    |
| `uptrace_mattermost_channel`     | `webhook_url`                                                                                                                    |
| `uptrace_teams_channel`          | `webhook_url`                                                                                                                    |
| `uptrace_pagerduty_channel`      | `routing_key`, `severity` (`critical`, `error`, `warning`, `info`)                                                               |
| `uptrace_opsgenie_channel`       | `api_key`, `priority` (`P1`-`P5`); optional `api_provider` (`opsgenie` default, `jsm`, `custom`), `endpoint` (for `custom`)         |
| `uptrace_telegram_channel`       | `chat_id` (int64)                                                                                                                |
| `uptrace_pushover_channel`       | `token`, `user_key`; optional `priority` (int, -2 to 2), `sound`                                                                |
| `uptrace_webhook_channel`        | `url`; optional `payload` (JSON string -- use `jsonencode()`)                                                                    |
| `uptrace_alertmanager_channel`   | `url`; optional `auth_method` (`none`, `basic_auth`, `bearer`), `username`, `password`, `token`                                  |
| `uptrace_incidentio_channel`     | `url`, `api_key`                                                                                                                 |
| `uptrace_servicenow_channel`     | `url`, `username`, `password`; optional `category`, `subcategory`, `impact` (`1`-`3`), `urgency` (`1`-`3`), `severity` (`1`-`5`), `caller_id`, `group`, `assigned_to`, `opened_by`, `notify` (`1` or `2`), `due_date` |

Import any channel: `terraform import uptrace_<type>_channel.x <project_id>:<channel_id>`

**Example -- Slack webhook:**

```hcl
resource "uptrace_slack_channel" "alerts" {
  project_id = uptrace_project.api.id
  name       = "alerts-slack"
  priorities = ["high", "medium"]

  auth_method = "webhook"
  webhook_url = "https://hooks.slack.com/services/T00/B00/XXXX"
}
```

### Monitors

Each monitor type is its own resource, as each has its own API route and params:

| Resource                    | Fires when                                                         |
|-----------------------------|--------------------------------------------------------------------|
| `uptrace_threshold_monitor` | the value leaves a fixed range (`min_value`, `max_value`)         |
| `uptrace_anomaly_monitor`   | the value leaves the range the detector learned for the series    |
| `uptrace_outlier_monitor`   | one series strays from its peers                                   |
| `uptrace_forecast_monitor`  | the value is projected to leave a range within `horizon`          |
| `uptrace_error_monitor`     | a new error group appears                                          |
| `uptrace_slo_monitor`       | an SLO rule fires, such as a burn rate                             |

**Shared fields:**

| Field             | Type        | Required | Description                                                                 |
|-------------------|-------------|----------|-----------------------------------------------------------------------------|
| project_id        | string      | yes      | Project ID. Forces replacement on change.                                   |
| name              | string      | yes      | Monitor name; may hold template variables such as `{{ .Attrs.host_name }}`. |
| description       | string      | no       | Alert description sent with each notification.                              |
| channel_ids       | set(string) | no       | Notification channel IDs. Omit to leave the links alone; `[]` detaches every channel. |
| trend_agg_func    | string      | no       | `sum`, `avg`, `median` or `last`. Server default when omitted.              |
| trend_sensitivity | string      | no       | `very_low` to `very_high`. Server default when omitted.                     |
| repeat_interval   | object      | no       | `{ strategy, interval, min, max }`: `default`, `fixed` (`interval`), `linear` or `exponential` (`min`, `max`). |
| id                | string      | computed |                                                                             |
| status            | string      | computed | `active`, `paused`, `firing`, `no_data`, `warming_up` or `disabled`.         |

Durations are strings such as `"90s"`, `"5m"` or `"30d"`.

A monitor and a channel share their links, so declare them on one side: a monitor's
`channel_ids` or a channel's `monitor_ids` (with `match_all = false`). The side that leaves its
list unset does not manage the links and shows no drift when the other side changes them.

**`params` of the metric-based monitors** (threshold, anomaly, outlier, forecast):

| Field             | Type            | Required | Description                                                       |
|-------------------|-----------------|----------|-------------------------------------------------------------------|
| metrics           | list of objects | yes      | `{ name, alias }`; the alias is `$` followed by an identifier.    |
| query             | string          | yes      | MQL expression that produces the monitored value.                 |
| column            | object          | no       | `{ name, unit }`: the result column to evaluate.                  |
| resolution        | string          | no       | Width of one evaluated point.                                     |
| num_eval_points   | number          | no       | How many recent points each check evaluates.                      |
| window_rule       | string          | no       | `at_all_times`, `at_least`, `on_average` or `in_total`.           |
| min_breach_points | number          | no       | Breaching points the `at_least` rule needs.                       |
| absent_points     | string          | no       | A missing point counts as `zero`, is `ignore`d, or breaches (`alert`). |
| time_offset       | string          | no       | How far back the evaluated window ends.                           |

Type-specific `params`:

- threshold: `min_value`, `max_value`, `recovery { min_value, max_value }`.
- anomaly: `preset`, `sensitivity`, `direction`, `seasonality`, `min_volume`, `min_change`.
- outlier: `min_peers`, `min_change`, `sensitivity`, `direction`, `keep_firing_for`.
- forecast: `min_value`, `max_value`, `seasonality`, `horizon`.
- error: only `metrics` and `query`.
- slo: `metrics`, `good_query`, `total_query`, `grouping`, `resolution`, `target`, `window_kind`,
  `rolling_window`, and `rules`, each `{ name, priority, type }` plus `long_window`, `short_window`,
  `factor` for `burn_rate` or `consumed_bound` for `budget_consumed`, and `min_total`.

The server normalizes an MQL query (an alias gains `{}`); the resource keeps the configured
spelling, so a refresh shows no diff.

Import any monitor: `terraform import uptrace_<type>_monitor.x <project_id>:<monitor_id>`. Importing
a monitor into the resource of another type fails and names the right one.

## Importing existing resources

To bring existing Uptrace resources under Terraform management, use `terraform import`.
The import ID format is listed under each resource above.

```bash
terraform import uptrace_org.main 123
terraform import uptrace_project.api 456
terraform import uptrace_slack_channel.alerts 456:789
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for building from source, running tests, and development
setup.
