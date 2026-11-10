# Contributing

Development guide for building and testing the Uptrace Terraform provider.

The provider is developed in the `terraform/` directory of the uptrace/app repository. It is a
separate Go module. github.com/uptrace/terraform-provider-uptrace is a mirror that the app CI
writes to; send changes to uptrace/app, not to the mirror. The commands below use
[Task](https://taskfile.dev). Run them in `terraform/`, or from the app root with the `terraform:`
prefix, for example `task terraform:test`.

## Building from source

```bash
go build -o terraform-provider-uptrace .
```

## Development setup

Create a dev override file so Terraform uses the local binary instead of downloading from the
registry:

```bash
cat > .terraformrc << 'EOF'
provider_installation {
  dev_overrides {
    "uptrace/uptrace" = "/path/to/terraform-provider-uptrace"
  }
  direct {}
}
EOF
```

Point Terraform at it:

```bash
export TF_CLI_CONFIG_FILE=/path/to/terraform-provider-uptrace/.terraformrc
```

## Testing

### Unit tests

Run without external dependencies:

```bash
task test
```

Covers pure mapping and request-building helpers, import paths, and ID validation. The
`build-terraform` job of the app CI runs them on each pull request that changes `terraform/`, and
on each push to `dev`.

### Acceptance tests

Exercise the full Terraform lifecycle (plan, apply, import, destroy) against a real Uptrace API.
Gated behind the `TF_ACC` environment variable.

1. Copy the credentials template and fill it in:

   ```bash
   cp .env.example .env
   ```

2. Run:

   ```bash
   task testacc
   ```

Without `TF_ACC=1`, acceptance tests are automatically skipped. `task testacc` downloads the
terraform CLI that `TF_VERSION` of `Taskfile.yml` pins; set `TF_ACC_TERRAFORM_PATH` to use a
local binary.

The `terraform-acceptance` job of the app CI runs them on each pull request that changes
`terraform/`, and on each push to `dev`. It builds a cloud server from the same commit, starts it
on a fresh test database (`config/test_cloud.default.yml`), and authenticates with the user token
that the config seeds. `.forgejo/ci/terraform-acceptance-config.cjs` gives the seeded org a
subscription, because the cloud build invites a user only for a member of a subscribed org.

### Writing tests

- **Unit tests** go in `*_unit_test.go` files with `package <name>` (internal). Use for pure
  functions that don't need a running API.
- **Acceptance tests** go in `*_acc_test.go` files with `package <name>_test` (external). Use the
  `TestAcc` prefix and `resource.TestCase` with `testutil.ProtoV6ProviderFactories`. Always include
  `PreCheck`, `CheckDestroy`, and an import step.

## Updating the API client

`generated/` is generated from `openapi/openapi.yaml`, a bundled copy of the spec that
lives in `openapi/` of the app. Vendor it from the app at a ref, then regenerate:

```bash
task sync-spec REF=origin/prod
task generate
```

`APP_DIR` names a different app checkout; the default is the parent directory.

Bundle from the oldest app version the provider must support. The server refuses a request key
its spec does not list, so a client built from a newer spec can send a key an older server
rejects. `openapi/overlay.yaml` adjusts the spec before generation (number types, keys that must
be sent as `null` or `[]`); its comments say why each action exists.

## Generating documentation

Registry documentation in `docs/` is generated from schema descriptions and the `examples/` directory:

```bash
task docs
```

Re-run this after changing resource schemas, descriptions, or example files, and commit the result.
It downloads the terraform CLI that `TF_VERSION` of `Taskfile.yml` pins, so each run reads the
schema through the same CLI.

## Files not in git

| File                         | What it is             | How to get it                              |
|------------------------------|------------------------|--------------------------------------------|
| `terraform-provider-uptrace` | Provider binary        | `go build -o terraform-provider-uptrace .` |
| `.terraformrc` / `.tofurc`   | Dev override config    | Create manually (see above)                |
| `*.tfstate`                  | Terraform state        | Created by `terraform apply`               |
| `.terraform/`                | Provider cache         | Created by `terraform init`                |
| `.terraform.lock.hcl`        | Dependency lock        | Created by `terraform init`                |
| `.env`                       | Local credentials      | `cp .env.example .env`                     |

## Releasing

See [RELEASING.md](RELEASING.md).
