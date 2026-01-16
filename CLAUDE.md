# CLAUDE.md

Context for Claude Code when working on Procyon.

## What is Procyon?

Procyon is a Go code generator for Trashtalk. It takes AST JSON from Trashtalk's jq-based parser and generates either Bash scripts or Go shared libraries that integrate with the Trashtalk runtime.

## Output Modes

Procyon supports two output modes:

### Bash Mode (`--mode bash`)
Generates optimized Bash scripts that are sourced by the Trashtalk runtime.

```bash
./driver.bash parse Counter.trash | procyon --mode bash > Counter.bash
```

### Shared Mode (`--mode shared`, default)
Generates Go code compiled to shared libraries (dylibs) loaded by trashtalk-daemon.

```bash
./driver.bash parse Counter.trash | procyon --mode shared > counter/main.go
cd counter && go build -buildmode=c-shared -o Counter.so .
```

## How the Pipeline Works

```bash
# Bash mode:
./driver.bash parse Counter.trash | procyon --mode bash > Counter.bash

# Shared library mode:
./driver.bash parse Counter.trash | procyon --mode shared > counter/main.go
go build -buildmode=c-shared -o ~/.trashtalk/plugins/Counter.so counter/main.go
```

1. `driver.bash parse` → tokenizes and parses .trash file → outputs AST JSON
2. `procyon` → reads AST JSON → generates Bash or Go code
3. For shared mode: `go build -buildmode=c-shared` → compiles to dylib
4. trashtalk-daemon loads dylibs on demand

## Key Design Decisions (May be out of date)

1. **Token stream parsing in Go**: The jq parser outputs method bodies as token streams, not expression trees. We parse these in Go rather than extending jq.

2. **Jennifer for codegen**: Using github.com/dave/jennifer for programmatic Go code generation.

3. **Two backends**: Bash for debugging/compatibility, shared libs for performance.

4. **trashtalk-daemon**: Loads shared libraries on demand

5. **libtrashtalk**: C runtime library providing instance storage and method registration.

## Issue Tracking

We use bd (beads) for issue tracking instead of Markdown TODOs or external tools.

### Quick Reference

```bash
# Find ready work (no blockers)
bd ready --json

# Find ready work including future deferred issues
bd ready --include-deferred --json

# Create new issue
bd create "Issue title" -t bug|feature|task -p 0-4 -d "Description" --json

# Create issue with due date and defer (GH#820)
bd create "Task" --due=+6h              # Due in 6 hours
bd create "Task" --defer=tomorrow       # Hidden from bd ready until tomorrow
bd create "Task" --due="next monday" --defer=+1h  # Both

# Update issue status
bd update <id> --status in_progress --json

# Update issue with due/defer dates
bd update <id> --due=+2d                # Set due date
bd update <id> --defer=""               # Clear defer (show immediately)

# Link discovered work
bd dep add <discovered-id> <parent-id> --type discovered-from

# Complete work
bd close <id> --reason "Done" --json

# Show dependency tree
bd dep tree <id>

# Get issue details
bd show <id> --json

# Query issues by time-based scheduling (GH#820)
bd list --deferred              # Show issues with defer_until set
bd list --defer-before=tomorrow # Deferred before tomorrow
bd list --defer-after=+1w       # Deferred after one week from now
bd list --due-before=+2d        # Due within 2 days
bd list --due-after="next monday" # Due after next Monday
bd list --overdue               # Due date in past (not closed)
```

### Workflow

1. **Check for ready work**: Run `bd ready` to see what's unblocked
2. **Claim your task**: `bd update <id> --status in_progress`
3. **Work on it**: Implement, test, document
4. **Discover new work**: If you find bugs or TODOs, create issues:
   - `bd create "Found bug in auth" -t bug -p 1 --json`
   - Link it: `bd dep add <new-id> <current-id> --type discovered-from`
5. **Complete**: `bd close <id> --reason "Implemented"`
6. **Export**: Run `bd export -o .beads/issues.jsonl` before committing

### Issue Types

- `bug` - Something broken that needs fixing
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature composed of multiple issues
- `chore` - Maintenance work (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (nice-to-have features, minor bugs)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Dependency Types

- `blocks` - Hard dependency (issue X blocks issue Y)
- `related` - Soft relationship (issues are connected)
- `parent-child` - Epic/subtask relationship
- `discovered-from` - Track issues discovered during work

Only `blocks` dependencies affect the ready work queue.

## Common Tasks

### Adding a new test case

1. Generate AST: `./driver.bash parse Foo.trash > testdata/foo/input.json`
2. Create expected output: `testdata/foo/expected.go`
3. Run tests: `go test ./pkg/codegen/...`

### Supporting a new token type

1. Add constant in `pkg/ast/types.go`
2. Handle in `pkg/parser/parser.go`
3. Generate code in `pkg/codegen/codegen.go`

### Adding a primitive class method

1. Find the appropriate file: `primitives_string.go`, `primitives_file.go`, `primitives_shell.go`, or `primitives.go`
2. Add a case to the switch statement in `generatePrimitiveMethod*`
3. Generate the Go implementation using jennifer

### Debugging codegen

The generated code should compile. If it doesn't:
1. Check `go test ./pkg/codegen/... -v` output
2. Look at the "ACTUAL" output in test failures
3. Try compiling the actual output: `procyon < input.json > test.go && go build test.go`

## Related Repositories

- **trashtalk** (`~/.trashtalk`): The main Trashtalk runtime and jq-based compiler
  - `lib/trash.bash`: Runtime dispatcher
  - `lib/jq-compiler/`: Tokenizer + parser + Bash codegen
