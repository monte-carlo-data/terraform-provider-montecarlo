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

So, **in the contents of any file committed here**: no internal file paths, no ticket
identifiers, and no design rationale that only makes sense from the inside. That reasoning is
valuable and should be written down — in the ticket, or in the internal repository that owns
generation. What belongs here is what a customer or an outside contributor can act on.

Two internal names are the exception: api-codegen, the generator, and monolith, the service
the API spec is exported from. Every generated file and `.api-codegen-source.json` already name
them, so naming them in prose adds nothing. Name no other internal repository.

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
go generate ./...   # regenerates docs/ and THIRD_PARTY_NOTICES; builds the provider and runs Terraform

go build -o . .     # writes ./terraform-provider-montecarlo, which dev_overrides needs

terraform -chdir=examples/resources/montecarlo_aws_collection_agent plan   # against that binary, see below
```

## Key Directories

| Path | Purpose |
|------|---------|
| `internal/provider/` | Generated resources and data sources, plus the hand-written helpers they call |
| `internal/schema_gen/` | Generated attribute schemas, one package per resource and per data source |
| `main.go` | Provider entry point |
| `examples/` | Worked configurations, which double as registry documentation |
| `templates/` | Registry page templates; only the index page has one |
| `docs/` | Registry documentation, generated — see [Registry documentation](#registry-documentation) |
| `tools/notices/` | Generates `THIRD_PARTY_NOTICES` — see [Third-party notices](#third-party-notices) |

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

Hand-written: `main.go`, the helpers the generated code calls, `tools/`, the examples and the
templates.

## Registry documentation

The registry renders `docs/` as committed; it does not read the schema. `go generate ./...` runs
`tfplugindocs`, at the version pinned in `main.go`, and rebuilds `docs/` from three inputs: the
schemas for every attribute description, `examples/` for each page's example, and `templates/`
for the index page, which adds the beta notice. Never edit `docs/` by hand. Change an input and
regenerate. CI fails when `docs/` differs from what `go generate` produces.

## Third-party notices

The release binaries compile in third-party modules, and their licenses require passing on
their license and notice files. `THIRD_PARTY_NOTICES` holds them, and every release zip carries
it next to `LICENSE` and `README.md`. `go generate ./...` rebuilds it with `tools/notices`, from
the modules `go list -deps` reports for every release platform. Never edit it by hand. CI fails
when it differs from what `go generate` produces, so a dependency change has to commit it too.

## Regeneration is automatic

api-codegen regenerates this repository and opens a pull request whenever it is behind the API
spec or behind `mc-sdk-go`. Nobody runs the generator from outside any more.

That pull request moves the SDK pin in the same commit as the code that needs it, which is the
only correct way to move it, and it has already been built and vetted against that pin before
being pushed. The generated paths carry no code owner, so it asks nobody for review — a person
still approves and merges it, and `go.mod` and `go.sum` are checked to make sure the bot changed
nothing there but the SDK's own lines.

`.api-codegen-source.json` at the root records what produced the tree: the api-codegen commit
and run, both `tfplugingen` versions, the `mc-sdk-go` commit pinned, and the monolith export the
spec came from. The next regeneration reads it to list what has changed since.

The regeneration runs `go generate ./...` last, so `docs/` moves in the same commit as the
schemas it documents, and `THIRD_PARTY_NOTICES` with the SDK pin.

Two things the bot leaves behind for whoever merges it. README.md's tables of every resource,
data source and import id are hand-written, so a pull request that adds a resource leaves them
stale. And a new resource ships with no `examples/` entry; those are written deliberately,
afterwards, rather than generated, so until then its registry page has no example.

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

**Not yet — the provider is unpublished.** Two properties make this a one-way door: the
provider address is permanent once published, and a published provider cannot be unpublished.

### How a release is built

Pushing a `v*` tag runs `.github/workflows/release.yml`. GoReleaser (`.goreleaser.yml`)
cross-compiles every platform, zips each one, writes the `SHA256SUMS` file with
`terraform-registry-manifest.json` in it, signs that file with GPG and attaches it all to a
GitHub release. The registry picks the release up from there.

The signing key is read from the `release` environment's secrets, `GPG_PRIVATE_KEY` and
`GPG_PASSPHRASE`, which only jobs running on a `v*` tag can use. The registry checks each release
against the public half of the same key.

The release refuses a tag whose commit is not on `main`, since that commit skipped review.

CI's `release-snapshot` job runs the same build, signed with a throwaway key and not published,
so a change that breaks a release fails before it merges. Tags are immutable, so a release that
fails after tagging burns its version. To reproduce it locally, without signing, using the
GoReleaser version pinned in `.github/actions/goreleaser/action.yml`:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.18.2 release --snapshot --clean --skip=sign
```

Releases stay `0.x` while the provider is in beta.

### Still outstanding before the first release

- **The Go SDK this provider depends on must be tagged.** It is public, but `go.mod` pins it to
  a pseudo-version because it has no tags. Tag it and replace the pseudo-version with the tag.
  Until then the pin is only as durable as the commit it names, though the public module proxy
  keeps serving any version it has already fetched, so a commit an upstream squash merge or
  history rewrite orphans still resolves once fetched.

  Moving the pin means regenerating, not `go get`. The SDK and the files under
  `internal/provider/` come from one spec export and move together. A constructor's parameters
  follow the order the spec declares the properties in. An export that changes that order
  changes signatures with no schema change. Two same-typed parameters swapping is invisible to
  the compiler, so a pin moved on its own can compile and pass the tests while sending a bucket
  name as a deployment id.
- **The signing key's public half, uploaded to the registry** when the provider is published.
  The private half and its passphrase are already in the `release` environment, which only `v*`
  tags can deploy to.
