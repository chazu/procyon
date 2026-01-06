# CLAUDE.md

Context for Claude Code when working on Procyon.

## What is Procyon?

Procyon is a Go code generator for Trashtalk. It takes AST JSON from Trashtalk's jq-based parser and generates native Go binaries that interoperate with the Bash runtime.

**Key insight**: This is an experiment. Bash remains the primary Trashtalk runtime. Native compilation is an optimization, not a replacement.

## Repository Structure

```
procyon/
├── cmd/procyon/main.go       # CLI - reads AST JSON from stdin, writes Go to stdout
├── pkg/
│   ├── ast/                  # Types matching jq parser output + JSON parsing
│   ├── parser/               # Token stream → expression tree (method bodies)
│   └── codegen/              # Jennifer-based Go code generator
├── testdata/counter/         # Acceptance test case
├── DESIGN.md                 # Full design document
└── README.md                 # User documentation
```

## How the Pipeline Works

```bash
# In trashtalk repo:
./lib/jq-compiler/driver.bash parse Counter.trash | procyon > counter/main.go
```

1. `driver.bash parse` → tokenizes and parses .trash file → outputs AST JSON
2. `procyon` → reads AST JSON → generates Go code using jennifer
3. `go build` → compiles to native binary
4. Binary placed in `~/.trashtalk/trash/.compiled/Counter.native`

## Key Design Decisions

1. **Token stream parsing in Go**: The jq parser outputs method bodies as token streams, not expression trees. We parse these in Go rather than extending jq.

2. **Jennifer for codegen**: Using github.com/dave/jennifer for programmatic Go code generation instead of text templates.

3. **Fallback mechanism**: Exit code 200 means "unknown selector" - the Bash dispatcher falls back to interpreted execution.

4. **Shared SQLite storage**: Both Bash and Go runtimes use `~/.trashtalk/instances.db` for instance persistence.

5. **Self-describing binaries**: Each binary embeds its source code and content hash via `//go:embed`.

## Current Capabilities (M1-M5)

**Compiles:**
- Instance variable access/assignment
- Local variable declarations (`| x y |`)
- Binary arithmetic (`+`, `-`, `*`, `/`)
- Comparison operators (`>`, `<`, `>=`, `<=`, `==`, `!=`)
- Control flow (`ifTrue:`, `ifFalse:`, `ifTrue:ifFalse:`)
- While loops (`whileTrue:`)
- Parenthesized expressions
- Return statements (`^`)
- Methods with arguments (string → int conversion)
- Self message sends (`@ self method`, `@ self keyword: arg`)
- External message sends (`@ OtherClass method`) via shell-out
- Namespaced classes (`package: MyApp` → `MyApp__Counter.native`)
- Class methods (`classMethod:` → package-level Go functions)

**Falls back to Bash:**
- `new` method (uses subshells)
- Trait methods (inlining not yet implemented)
- Raw methods
- Subshell expressions (`$(...)`)

## Testing

```bash
go test ./pkg/codegen/... -v    # Acceptance tests
go test ./...                    # All tests
```

Acceptance tests compare generated code against `testdata/*/expected.go`.

## Next Steps (M6)

See DESIGN.md for the full roadmap. Key upcoming work:

1. **Polish (M6)**: Better error messages, `--strict` mode improvements, documentation

**Note**: Trait method inlining is deferred - existing traits use Bash-specific constructs that can't compile to Go. See docs/trait-inlining.md for the planned approach when needed.

## Common Tasks

### Adding a new test case

1. Generate AST: `./driver.bash parse Foo.trash > testdata/foo/input.json`
2. Create expected output: `testdata/foo/expected.go`
3. Run tests: `go test ./pkg/codegen/...`

### Supporting a new token type

1. Add constant in `pkg/ast/types.go`
2. Handle in `pkg/parser/parser.go`
3. Generate code in `pkg/codegen/codegen.go`

### Debugging codegen

The generated code should compile. If it doesn't:
1. Check `go test ./pkg/codegen/... -v` output
2. Look at the "ACTUAL" output in test failures
3. Try compiling the actual output: `procyon < input.json > test.go && go build test.go`

## Related Repositories

- **trashtalk** (`~/.trashtalk`): The main Trashtalk runtime and jq-based compiler
  - `lib/trash.bash`: Runtime dispatcher (has native binary support)
  - `lib/jq-compiler/`: Tokenizer + parser + Bash codegen
  - `docs/trashtalk-go-codegen-design.md`: Original design doc
