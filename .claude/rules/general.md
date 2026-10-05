---
description: Coding standards for this repo
paths: "**/*.go"
---

# General Standards

Most of the Go here is generated — the resources, the data sources, the provider registration and
every attribute schema, each carrying a `DO NOT EDIT` header. The hand-written surface is
`main.go`, `tools/` and the helpers under `internal/provider/` that the generated code calls.
These rules apply to that surface. Do not hand-edit a generated file: the next generation run overwrites it, so the fix
belongs in the API or in the generator that reads its spec.

Which side a *new* file falls on is decided by its path and name alone, so know the rule before
adding one:

- `internal/schema_gen/` is deleted whole and recreated. Nothing hand-written can live there.
- In `internal/provider/`, exactly `*_resource.go`, `*_data_source.go` and `provider.go` are
  replaced. Every other name in that directory is left alone.

So a hand-written `internal/provider/thing_resource.go` is lost on the next regeneration, while
`internal/provider/thing.go` survives — and the loss is silent, arriving as a deletion in the
regeneration diff that nobody made. AGENTS.md carries the same rule with the full context.

## Code Style

- Keep functions small and focused on a single responsibility
- Prefer explicit over implicit — name things clearly
- Write code that reads like a specification
- Exported identifiers need a doc comment beginning with the identifier's own name — that is what
  makes `go doc` and pkg.go.dev read correctly
- A helper the generated code calls is half of a contract with the generator. Its name and
  signature are what the templates emit, so changing either is a coordinated change across two
  repositories, not a local one
- Attribute names and descriptions become the provider's registry documentation. Write them for a
  practitioner reading them in the registry, not for a maintainer reading them here

## Testing

- Tests are co-located as `<file>_test.go` beside the source, per Go convention, and because the
  package's own tests reach unexported identifiers — `internal/provider/helpers_test.go` covers
  `retryableStatus`, `applyPlanModifier` and `requiresReplace`, none of which are exported
- Use `package provider` for a test that reaches unexported identifiers, and `package
  provider_test` for a black-box test of what the provider exposes
- Write tests for all non-trivial logic
- Name a test for the invariant it pins, not for the function it calls
- Prefer the real boundary to a mock. The provider's boundary is the Monte Carlo API over HTTP, so
  drive it with `httptest.NewServer` and a real client rather than a hand-written fake
- An acceptance test drives Terraform itself through `helper/resource`, is co-located with the
  resource it exercises, and is gated on `TF_ACC` so that `go test ./...` stays offline
- A helper that switches over the framework's attribute types is only as correct as its coverage,
  and a missing case surfaces only when someone plans against that resource. Pin every type the
  switch must handle in a table test instead of trusting the switch to be complete

## Error Handling

- Handle errors at system boundaries — the API response, and whatever Terraform hands the provider
  from a practitioner's configuration
- Report a failure through `diag.Diagnostics` rather than returning it. That is the only channel
  Terraform surfaces to the practitioner, and a resource method has nowhere else to put it
- A diagnostic says what to change. An API failure carries the response body with it, because the
  status alone does not name the attribute that was at fault
- Wrap with `%w` and match with `errors.Is`/`errors.As`; do not compare error strings
- A mistake the generated code could make — an attribute that is not in the schema, an attribute
  type with no plan modifier — is reported, not skipped. Skipping restores the silent behaviour the
  caller attached the modifier to prevent
- Don't add error handling for scenarios that can't happen

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` (must be empty), and `go test -race ./...`. CI runs
all four on every pull request and on every push to `main`.
