---
version: 4
skill: docs-curate
scope: [terraform/]
---

# Terraform provider

## What

The Terraform provider lets an operator manage the organizations, teams, projects, users,
tokens, notification channels and monitors of Uptrace as Terraform resources. The operator
installs the provider from the public Terraform Registry and declares the resources in HCL.
Terraform then plans, applies, imports and destroys them through the Uptrace API. A
maintainer of the app changes the provider in the same repository and the same review as
the API that it calls.

## Why

The Terraform Registry reads only a public GitHub repository with a fixed name, and it reads
the documentation from the tagged commit. The app repository is private, and its API
changes in each release. The provider must work with the servers that operators run, which
can be older than the newest release. Terraform also fails an apply when the state that the
server returns differs from the plan, so each value that the server rewrites must be known
before the plan. A provider kept apart from the app drifts from the API without a signal,
and a provider built from the newest API refuses older servers.

## How

The provider is a separate Go module in `terraform/` that does not import the app module
(D3). Its API client is generated from a copy of the app's OpenAPI contract, which a
maintainer bundles from the oldest app version that the provider supports (D2). The app CI
builds, lints and tests the module in its own job, and builds it against the contract of
each pull request that changes the API, as an advisory check (D4). A workflow of the app writes each change
of `terraform/` to the public GitHub repository as a snapshot commit, and pushes the release
tags there; the release workflow of that repository builds and signs the release (D1).

## An operator installs the provider from the registry

### Each release is served under the address uptrace/uptrace (G1, consumer, contract)

Each operator configuration names the provider by that address. A release under another
address, or with other artifact names, does not install, and the registry does not list it.

### A release holds the binaries and docs of the tagged tree, and no other tree (G2, consumer)

An operator reads the docs of a version on the registry and runs the binaries of that
version. When the two come from different trees, the docs describe attributes that the
binary does not have.

### D1 — The app CI writes the public repository as a fast-forward snapshot mirror

Serves: G1, G2, G5
Trades: none

**Decision.** The app is the only source, and a workflow of the app appends one snapshot
commit of `terraform/` to the public repository for each change, because the registry reads
only that repository and a second source of changes would diverge. A release tag of the app
pushes the registry tag at the snapshot of the tagged tree. The mirror only fast-forwards:
a commit that reached it outside the app stops the sync, and a tag is never moved.

**Alternatives.** *Split the history with `git subtree split`.* It keeps one public commit
for each app commit, but it walks the full app history on each run, and a squash merge of
the app already hides the commits it would keep. *Develop in the public repository and
vendor it into the app.* It keeps outside pull requests, but each API change then needs two
pull requests and two reviews, which is the drift this design removes.

**Consequences.** An outside contributor cannot open a pull request that merges on GitHub;
a maintainer moves the change into the app. The public history holds one commit for each
sync, not each app commit.

**What would reopen it.** The registry accepts a source other than a public GitHub
repository, or outside contributions become frequent enough that a second review is cheaper
than the move.

## Terraform shows in the plan what the server stores

### A plan shows only values that the server stores, so an apply never fails with an inconsistent result (G3, consumer)

Terraform fails an apply halfway when the state that the server returns differs from the
plan. The operator then has a partly applied change and a state to repair by hand. A value
that the server rewrites or refuses must fail the plan instead.

### The provider works with each app version from the oldest one that it supports (G4, consumer)

Operators upgrade their servers on their own schedule. The server refuses a request key that
its contract does not list, so a client built from a newer contract can send a key that an
older server refuses.

*Unchecked:* no test runs the provider against an older server. A maintainer bundles the
contract from the oldest supported version, as `terraform/CONTRIBUTING.md` states. The
acceptance tests of CI run against a server of the same commit, which is the newest version.

### D2 — Generate the client from a bundled copy of the oldest supported contract

Serves: G4
Trades: none

**Decision.** The client is generated from `openapi/openapi.yaml` of the provider, a bundle
of the app contract at the oldest supported version, because the generated client sends
only the keys that the bundled contract lists, and the oldest server must accept each one.

**Alternatives.** *Generate from `openapi/` of the same commit.* It catches an API change
that breaks the provider in the same pull request, but the provider would then send keys
that each older server refuses.

**Consequences.** A new API field reaches the provider only when a maintainer bundles the
contract again. A change of the app contract that breaks the provider does not fail the
pull request that made it: it shows there as the advisory check of D4, and the maintainer
repairs the provider at the next sync.

**What would reopen it.** The server accepts unknown request keys, or the provider supports
only the newest app version.

### D4 — CI builds the provider against the contract of each pull request that changes openapi/, as an advisory check

Serves: G4
Trades: none

**Decision.** A job of the app CI bundles `openapi/` of the pull request into a copy of the
module, generates the client there and builds and tests the provider, because a change that
breaks the provider is cheapest to see in the pull request that makes it. The job turns
red, but no other job waits for it, because D2 keeps the provider on an older contract and a
break of the newest one does not break a release.

**Alternatives.** *Block the pull request.* It stops the break before the merge, but each
API change would then have to sync the provider at once, which D2 rejects. *Check only at
the next sync.* It costs no CI time, but the maintainer then finds the break far from the
change that caused it.

**Consequences.** Each pull request that changes `openapi/` runs one more job. A red
advisory job can stay red on a pull request that merges, so a person must read it rather
than trust the color of the run.

**What would reopen it.** The provider supports only the newest app version (D2 reopens),
so the check must block; or the job stays red so often that nobody reads it.

## A change to the provider gets the review and CI of the app

### A pull request that changes terraform/ runs each check of the provider module (G5, user)

The user moved the provider into the app so that its changes get the same review and CI as
the app. A check that the pull request skips finds a defect only after the release.

### D3 — The provider is a separate Go module that does not import the app

Serves: G1, G5
Trades: none

**Decision.** `terraform/` holds its own `go.mod` and imports no package of the app module,
because the public mirror must build without the private app, and the provider must not
carry the app's dependencies into each operator binary.

**Alternatives.** *A package of the app module.* It lets the provider share the app's API
types and its tests, but the mirror then cannot build, and each app dependency enters the
provider binary.

**Consequences.** No Go lane of the app reaches the module, so it needs a CI job of its own,
and each walk over the app's Go code must skip it. The provider cannot reuse a helper of
the app; it copies the few that it needs.

**What would reopen it.** The app module becomes public and small enough to import, or the
registry starts to build providers from a monorepo path.

## Frozen contracts

### The registry address registry.terraform.io/uptrace/uptrace, the tags vX.Y.Z of the mirror, and the release artifact names terraform-provider-uptrace_{version}_{os}_{arch}.zip, _SHA256SUMS, _SHA256SUMS.sig and _manifest.json (F1)

The Terraform Registry finds a provider release by the repository, the tag and these file
names, and the manifest states protocol version 6.0.

*Frozen by:* the provider publishing protocol of the Terraform Registry, and the
`required_providers` source of each operator configuration. *A break costs:* operators
cannot install new versions, or the registry does not list them.

### The resource type names, their attribute names and their import ID formats (F2)

Each resource is named `uptrace_<name>`, and the docs under `terraform/docs/` list the
attributes and the import IDs.

*Frozen by:* operator configurations and the Terraform state that operators keep. *A break
costs:* a renamed type or attribute makes Terraform plan a destroy and a create, or fail to
read the state; a changed import ID breaks the import commands of operators.

## Decisions

- **D1 — The app CI writes the public repository as a fast-forward snapshot mirror** — reopens when the registry accepts another source, or outside contributions grow.
- **D2 — Generate the client from a bundled copy of the oldest supported contract** — reopens when the server accepts unknown keys, or only the newest version is supported.
- **D4 — CI builds the provider against the contract of each pull request that changes openapi/, as an advisory check** — reopens when only the newest version is supported, or the job stays red so often that nobody reads it.
- **D3 — The provider is a separate Go module that does not import the app** — reopens when the app module can be imported, or the registry builds from a monorepo path.
