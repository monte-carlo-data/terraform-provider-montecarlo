# terraform-provider-montecarlo

> The official Terraform provider for Monte Carlo. Published to the Terraform Registry as `monte-carlo-data/montecarlo`, so HCL reads `provider "montecarlo"` and resources are `montecarlo_deployment` and so on.

## This repository is public

Treat everything here as customer-facing, including things that are easy to forget:

- **Commit messages and pull request descriptions.** A closed pull request stays visible, a
  force-pushed commit stays reachable by its sha, and edited descriptions keep an edit
  history. None of it can be deleted after the fact.
- **HCL attribute names and their descriptions**, which become the provider's registry
  documentation.
- **Generated code**, which ships as-is.

So: no internal repository names, no internal file paths, no ticket identifiers, and no
design rationale that only makes sense from the inside. That reasoning is valuable and should
be written down — in the Linear ticket, or in the internal repository that owns generation.
What belongs here is what a customer or an outside contributor can act on.

The generated-file header naming the generator is deliberate and stays: it tells a maintainer
where a file came from, which is the same reason `protoc-gen-go` and `openapi-generator` do it.

## Stack

- **Language:** Go 1.23+
- **Framework:** terraform-plugin-framework
- **Client:** the Monte Carlo Go SDK, which supplies the API client and its authentication

## Common Commands

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .          # must be empty

terraform -chdir=examples/... plan   # against a locally built binary, see below
```

## Key Directories

| Path | Purpose |
|------|---------|
| `internal/provider/` | Generated resources and data sources, plus the hand-written helpers they call |
| `internal/schema_gen/` | Generated attribute schemas, one package per resource |
| `main.go` | Provider entry point |
| `examples/` | Worked configurations, which double as registry documentation |

## What is generated

The resources, the data sources, the provider registration and every attribute schema. A fix
to one of those files does not survive regeneration — it belongs in the API, or in the
generator that reads its spec.

Hand-written: `main.go`, the helpers the generated code calls, and the examples.

## Running a locally built provider

`dev_overrides` in `~/.terraformrc` points Terraform at a local binary, which needs no
registry, no signing and no release:

```hcl
provider_installation {
  dev_overrides {
    "monte-carlo-data/montecarlo" = "/path/to/your/gopath/bin"
  }
  direct {}
}
```

`terraform init` fails under `dev_overrides` — run `plan` and `apply` directly. Lock files are
ignored while it is in effect.

## Branching

Branch from `main` as `<person>/<ticket-id>-<slug>`. Never commit directly to `main`.

## Releasing

**Not yet.** Publishing to the registry needs a GPG key, a signed release and a manifest, and
the provider address is permanent once published.
