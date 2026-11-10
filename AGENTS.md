# Terraform Provider Development

Rules here apply to `terraform/`, the Terraform provider. It is a separate Go module, and
the app CI copies it to the public GitHub repository that the Terraform Registry reads.
`README.md` is the user guide, `CONTRIBUTING.md` holds the commands, and `DESIGN.md` says why
the module is shaped this way.

## What differs from the root AGENTS.md

- **Import no package of `github.com/uptrace/uptrace`.** The public mirror builds without the
  app, so such an import does not build there (`DESIGN.md` D3). Copy a small helper instead.
- **The root rules that name an app package do not apply.** The module cannot import
  `pkg/json`, `pkg/validerr`, `pkg/httperr`, `pkg/httpkit`, `pkg/pg` or `pkg/fx`. Use
  `encoding/json`, the diagnostics and validators of the plugin framework, and the generated
  client in `generated/`. The rules on `pkg/api`, `api_out.go`, migrations, build tags and
  `featflag` are about the app and do not apply either.
- **Do not edit `generated/` or `docs/`.** Run `task generate` after a change to
  `openapi/openapi.yaml`, `openapi/overlay.yaml` or `oapi-codegen.yaml`, and `task docs` after a
  change to a schema description or to `examples/`. Commit the result. The `build-terraform`
  job fails when either one is stale.
- **Bundle the contract from the oldest supported app version.** Run
  `task sync-spec REF=<ref>` from this directory, not from the newest commit: the server
  refuses a request key that its contract does not list (`DESIGN.md` D2).
- **Tests follow `CONTRIBUTING.md` § Writing tests**, not `docs/go-test-conventions.md`: a unit
  test sits in a `*_unit_test.go` file of the package; an acceptance test sits in a
  `*_acc_test.go` file of the external test package, starts with `TestAcc`, and holds
  `PreCheck`, `CheckDestroy` and an import step. The acceptance tests run only with
  `TF_ACC=1`.
- **A resource type name, an attribute name and an import ID format are frozen**
  (`DESIGN.md` F2). A rename ships a state upgrade and a major version.
- **Run the checks from this directory**: `task test`, `task vet`, `task lint`,
  `task fmt:check`. No lane of the root `task test` reaches this module. Before a pull request
  that changes only `terraform/`, run these and the root package tests,
  `go test -short -count=1 .` at the app root, in place of the root `task test`: the root
  package tests read `DESIGN.md` and the files that it cites.
- **Release with a tag `terraform/vX.Y.Z` on a commit of `dev`**, as `RELEASING.md` states.
  Never push to the GitHub mirror by hand.
