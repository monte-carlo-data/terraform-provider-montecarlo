# terraform-provider-montecarlo

> The official Terraform provider for Monte Carlo. It is **not published to the Terraform Registry yet** (see [Releasing](#releasing)); the address it will take is `monte-carlo-data/montecarlo`, so HCL reads `provider "montecarlo"` and resources are `montecarlo_deployment` and so on. Until it is published, run it locally — see [Running a locally built provider](#running-a-locally-built-provider).

## This repository is public

Treat everything here as customer-facing, including things that are easy to forget:

- **Commit messages and pull request descriptions.** A closed pull request stays visible, a
  force-pushed commit stays reachable by its sha, and edited descriptions keep an edit
  history. None of it can be deleted after the fact.
- **HCL attribute names and their descriptions**, which become the provider's registry
  documentation.
- **Generated code**, which ships as-is.

So, **in the contents of any file committed here**: no internal repository names, no internal
file paths, no ticket identifiers, and no design rationale that only makes sense from the
inside. That reasoning is valuable and should be written down — in the ticket, or in the
internal repository that owns generation. What belongs here is what a customer or an outside
contributor can act on.

Branch names and pull request metadata are the exception, and they may carry a username and a
ticket id: `<person>/<ticket-id>-<slug>` is the convention below, it matches the organisation's
other public repositories, and a merged pull request displays its head branch permanently. So
the prohibition above is about what files say, not about what a branch is called.

The generated-file header naming the generator is deliberate and stays: it tells a maintainer
where a file came from, which is the same reason `protoc-gen-go` and `openapi-generator` do it.

## Stack

- **Language:** Go 1.25+, as `go.mod`'s `go` directive requires. That directive is the
  authoritative floor; this line follows it
- **Framework:** terraform-plugin-framework
- **Client:** the Monte Carlo Go SDK, which supplies the API client and its authentication

## Common Commands

```bash
go build ./...      # compile check only — it produces no binary, see below
go test ./...
go vet ./...
gofmt -l .          # must be empty

go build -o . .     # writes ./terraform-provider-montecarlo, which dev_overrides needs

terraform -chdir=examples/aws-agent plan   # against that binary, see below
```

## Key Directories

| Path | Purpose |
|------|---------|
| `internal/provider/` | Generated resources and data sources, plus the hand-written helpers they call |
| `internal/schema_gen/` | Generated attribute schemas, one package per resource and per data source |
| `main.go` | Provider entry point |
| `examples/` | Worked configurations, which double as registry documentation |

## What is generated

The resources, the data sources, the provider registration and every attribute schema. A fix
to one of those files does not survive regeneration — it belongs in the API, or in the
generator that reads its spec.

Regeneration decides what to replace **by filename**, not by reading the file, so the boundary
is a naming rule and it is worth knowing exactly:

- `internal/schema_gen/` is deleted whole and recreated. Nothing hand-written can live there.
- In `internal/provider/`, exactly `*_resource.go`, `*_data_source.go` and `provider.go` are
  deleted and rewritten. Every other file in that package is left untouched.

A new hand-written file under `internal/schema_gen/`, or one in `internal/provider/` whose name
matches those patterns, is therefore lost on the next regeneration, and the loss looks like a
deletion nobody made. Hand-written code in `internal/provider/` has to be named so it falls
outside the patterns, as `helpers.go` and `helpers_test.go` do.

Hand-written: `main.go`, the helpers the generated code calls, and the examples.

## Running a locally built provider

`dev_overrides` in `~/.terraformrc` points Terraform at a local binary, which needs no
registry, no signing and no release. Two steps: build the binary, then point Terraform at the
directory holding it.

```bash
go build -o . .     # writes ./terraform-provider-montecarlo
```

`go build ./...` is a compile check, not a build: when the pattern matches more than one
package Go discards the executables, so it leaves no binary anywhere. Build the module's root
package explicitly, as above. `.gitignore` already ignores the resulting repo-root binary.

`dev_overrides` takes the **directory** the binary is in, not the binary itself:

```hcl
provider_installation {
  dev_overrides {
    # The directory the binary was built into — the repository root, if you followed the above.
    "monte-carlo-data/montecarlo" = "/absolute/path/to/your/clone/terraform-provider-montecarlo"
  }
  direct {}
}
```

`terraform init` fails under `dev_overrides` — run `plan` and `apply` directly. Lock files are
ignored while it is in effect. If Terraform reports `no provider ... plugin found in
dev_overrides directory`, the binary is missing: re-run the `go build -o . .` above.

## Branching

Branch from `main` as `<person>/<ticket-id>-<slug>`. The username and ticket id are deliberate
and permitted here, per the carve-out above. Never commit directly to `main`.

## Releasing

**Not yet — the provider is unpublished, and every item below is still outstanding.** Two
properties make this a one-way door: the provider address is permanent once published, and a
published provider cannot be unpublished.

Before the first release:

- **The Go SDK this provider depends on must be public and tagged.** `go.mod` pins it to a
  pseudo-version because it has no tags. If it is still private when this repository goes
  public, `go build ./...` fails for every outside contributor and for any registry build from
  source — so this is a hard precondition, not a nice-to-have. Publish it, tag it, and replace
  the pseudo-version with the tag. Until then the pin is only as durable as the commit it names:
  an upstream squash merge or history rewrite orphans that commit, and because the module
  resolves directly from git rather than through a proxy, every clean clone and every CI run
  then fails at module download. A green local build does not disprove it — the module cache
  still holds the orphaned version — so after any upstream merge, confirm the pinned commit is
  still reachable from the SDK's default branch and re-pin if it is not.
- **A GPG key**, with the private half held as a repository or organisation secret and the
  public half uploaded to the registry. The registry verifies the signature on every release.
- **`.goreleaser.yml`**, which builds the per-platform archives, the `SHA256SUMS` file and its
  detached signature in the layout the registry requires.
- **`terraform-registry-manifest.json`**, declaring the protocol version.
- **A tag-triggered release workflow** that runs GoReleaser and attaches the artefacts to the
  GitHub release. Nothing produces a release today.
- **`docs/`**. The registry renders provider documentation from markdown under `docs/` — it
  does **not** derive it from the schema, so publishing without it yields a page with attribute
  descriptions and no resource documentation. `tfplugindocs` generates it from the schemas plus
  `examples/`, which is the other reason `examples/` exists.
