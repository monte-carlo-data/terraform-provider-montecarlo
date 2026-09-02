---
description: General coding standards for this repo
globs: "**/*"
---

# General Standards

<!-- Add repo-specific coding standards here. Examples below. -->

## Code Style

- Keep functions small and focused on a single responsibility
- Prefer explicit over implicit — name things clearly
- Write code that reads like a specification

## Testing

- Write tests for all non-trivial logic
- Tests live in `tests/` mirroring the source structure
- Prefer integration tests over mocks at system boundaries

## Error Handling

- Handle errors at system boundaries (user input, external APIs)
- Don't add error handling for scenarios that can't happen
- Let internal errors propagate — don't swallow exceptions silently
