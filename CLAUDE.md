# CLAUDE.md

Context for Claude Code when working on Procyon.

## What is Procyon?

Procyon is a Go code generator for Trashtalk. It takes AST JSON from Trashtalk's jq-based parser and generates either Bash scripts or Go shared libraries that integrate with the Trashtalk runtime.

**Key insight**: This is an experiment. Bash remains the primary Trashtalk runtime. Native compilation is an optimization, not a replacement.

## Repository Structure

```
procyon/
├── cmd/
│   ├── procyon/           # CLI - reads AST JSON from stdin, writes code to stdout
│   ├── trashtalk-daemon/  # Shared library loader daemon
│   └── libtrashtalk/      # C runtime library
├── lib/
│   └── runtime/           # libtrashtalk headers
├── pkg/
│   ├── ast/               # Types matching jq parser output + JSON parsing
│   ├── parser/            # Token stream → expression tree (method bodies)
│   ├── ir/                # Intermediate representation
│   ├── codegen/           # Code generators (Bash backend, Shared lib mode)
│   ├── bytecode/          # Block VM for closure execution
│   └── runtime/           # Go runtime helpers
├── testdata/              # Acceptance test cases
├── DESIGN.md              # Full design document
└── README.md              # User documentation
```

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

## Key Design Decisions

1. **Token stream parsing in Go**: The jq parser outputs method bodies as token streams, not expression trees. We parse these in Go rather than extending jq.

2. **Jennifer for codegen**: Using github.com/dave/jennifer for programmatic Go code generation.

3. **Two backends**: Bash for debugging/compatibility, shared libs for performance.

4. **trashtalk-daemon**: Loads shared libraries on demand, dispatches via JSON protocol over Unix socket.

5. **libtrashtalk**: C runtime library providing instance storage and method registration.

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
- External message sends (`@ OtherClass method`)
- Namespaced classes (`package: MyApp`)
- Class methods (`classMethod:` → package-level Go functions)
- Primitive classes (String, File, Shell, Env, Console, Array, Dictionary, GrpcClient)

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
