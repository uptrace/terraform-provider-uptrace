# Releasing

This document describes how to publish a new version of the Terraform Uptrace
provider to the Terraform Registry.

The provider is developed in `terraform/` of the uptrace/app repository. The
Terraform Registry reads only the public GitHub repository
github.com/uptrace/terraform-provider-uptrace. The `terraform-mirror` workflow of
uptrace/app (`.forgejo/workflows/terraform-mirror.yml`) writes that repository:

- Each push to `dev` of uptrace/app adds a snapshot commit of `terraform/` to the
  `dev` branch of the mirror, unless `terraform/` did not change.
- Each tag `terraform/vX.Y.Z` of uptrace/app pushes the tag `vX.Y.Z` to the mirror,
  at the snapshot of the tagged commit.

The **Release** workflow of the mirror (`.github/workflows/release.yml`) starts on
that tag. It builds the binaries with GoReleaser, signs the checksum file, and
creates a **draft** GitHub release.

Do not push commits or tags to the mirror by hand. The next sync refuses to
write over a commit that it did not make, and the mirror stays stale until a
person moves the change into uptrace/app.

## One-time setup

### 1. Generate a GPG signing key

The Terraform Registry requires release artifacts to be signed with GPG.

```bash
gpg --full-generate-key
```

Choose RSA 4096-bit with no expiration (or your preferred expiration). Use an
email associated with your GitHub account.

### 2. Add the public key to the Terraform Registry

Export the public key and add it to the provider's signing keys page on the
Terraform Registry:

```bash
gpg --armor --export <key-id>
```

See https://developer.hashicorp.com/terraform/registry/providers/publishing#preparing-and-adding-a-signing-key

### 3. Configure the mirror secrets

In the GitHub repository settings of the mirror under **Settings > Secrets and
variables > Actions**, add the following secrets:

| Secret            | Value                                                        |
| ----------------- | ------------------------------------------------------------ |
| `GPG_PRIVATE_KEY` | Armor-exported private key (`gpg --armor --export-secret-keys <key-id>`) |
| `PASSPHRASE`      | Passphrase for the GPG key                                   |

### 4. Configure the uptrace/app secret

In the Forgejo settings of uptrace/app, add the secret `TERRAFORM_MIRROR_TOKEN`:
a GitHub token that may push contents (branches and tags) to the mirror. Use a
fine-grained personal access token or an app token. The `GITHUB_TOKEN` of a
GitHub run does not work: a tag that it pushes starts no workflow.

## Creating a release

1. Re-generate registry documentation and commit any changes through a pull
   request:

   ```bash
   task terraform:docs
   ```

2. Make sure all changes are merged to `dev` of uptrace/app, CI is green, and
   the `terraform-mirror` run of the merge passed.

3. Choose a version following [Semantic Versioning](https://semver.org/):
   - **patch** (v0.1.1) — bug fixes
   - **minor** (v0.2.0) — new resources or data sources, new attributes
   - **major** (v1.0.0) — breaking changes to existing resources or attributes

4. Create and push an annotated tag on a commit of `dev` of uptrace/app:

   ```bash
   git tag -a terraform/v0.4.0 -m "terraform/v0.4.0" origin/dev
   git push origin terraform/v0.4.0
   ```

5. The `terraform-mirror` run of the tag pushes `v0.4.0` to the mirror. It fails
   when the tagged commit is not on `dev`, or when the mirror already has the tag.

6. The **Release** workflow of the mirror creates a draft release. Review it on
   the GitHub releases page, edit the release notes if needed, then publish it.

7. The Terraform Registry detects the published release via webhook and makes
   the new version available.

## Verifying

After the release is published, confirm the version appears on the registry:

```
https://registry.terraform.io/providers/uptrace/uptrace/latest
```

You can also verify locally:

```bash
terraform init -upgrade
terraform providers
```
