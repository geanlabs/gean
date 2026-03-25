# AGENTS

## Guidelines

- **Keep changes minimal and focused.** Only modify code directly related 
  to the task.
- **Do not add dependencies** unless explicitly required.
- **Write tests** for new functionality.

## Pre-Commit Checklist

Before every commit, run all of the following:
```sh
make fmt            # Format code
make lint           # Lint checks
make unit-test      # Unit tests
make spec-test      # Spec tests
```

All checks must pass before committing.

## Commit Message Format

`<scope>: description`

Examples:
- `forkchoice: add attestation weight tracking`
- `statetransition: fix slot gap validation`

Rules:
- Use present tense: "add" not "added", "fix" not "fixed"
- Scope should match the package or component being changed
  (e.g. `forkchoice`, `statetransition`, `network`, `validator`)
- Keep the description short and lowercase
