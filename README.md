<p align="center">
  <img src="https://github.com/chazu/procyon/blob/main/img/logo.png">
</p>

# Procyon

*Procyon: the genus for raccoons - because what goes better with trash?*

Procyon is a Go code generator for [Trashtalk](https://github.com/chazu/trashtalk), a Smalltalk-inspired DSL that compiles to Bash. It takes the AST output from Trashtalk's jq-based parser and generates either Bash scripts or Go shared libraries that integrate with the Trashtalk runtime.

## Status

**M5 Complete** - Full code generation working with two backends.

- **Bash backend**: Generates optimized Bash scripts from Trashtalk AST
- **Shared library mode**: Generates Go dylibs loaded by trashtalk-daemon for native performance

## Installation

```bash
go install github.com/chazu/procyon/cmd/procyon@latest
```

Or build from source:

```bash
git clone https://github.com/chazu/procyon
cd procyon
go build -o procyon ./cmd/procyon
```

## Usage

Procyon reads AST JSON from stdin and writes code to stdout:

```bash
# Generate Bash script
./driver.bash parse Counter.trash | procyon --mode bash > Counter.bash

# Generate Go shared library source
./driver.bash parse Counter.trash | procyon --mode shared > counter/main.go

# Build the shared library
cd counter && go build -buildmode=c-shared -o Counter.so .
```

### CLI Options

```
procyon [options] < ast.json > output

Options:
  --mode string     Output mode: bash or shared (default "shared")
  --strict          Fail on unsupported constructs instead of warning
  --dry-run         Show what would be generated without outputting
  --source-file     Path to original source file for embedding (bash mode)
  --skip-vet        Skip Go validation of generated code (shared mode)
  --version         Print version and exit
```

### Output Modes

| Mode | Output | Use Case |
|------|--------|----------|
| `bash` | Bash script | Direct execution, debugging |
| `shared` | Go source for dylib | Native performance via trashtalk-daemon |

### Generation Report

Procyon reports which methods were compiled and which will fall back to Bash:

```
procyon: Counter.trash
  ⚠ new - skipped: subshell expressions not supported
  ✓ getValue - compiled
  ✓ getStep - compiled
  ✓ setValue_ - compiled
  ✓ increment - compiled

Generated 4/5 methods. 1 will fall back to Bash.
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         trashtalk repo                          │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │ .trash file │───▶│ tokenizer   │───▶│ parser.jq           │ │
│  └─────────────┘    │ (bash)      │    │ (outputs AST JSON)  │ │
│                     └─────────────┘    └──────────┬──────────┘ │
└──────────────────────────────────────────────────│─────────────┘
                                                   │
                                                   ▼ AST JSON (stdin)
┌─────────────────────────────────────────────────────────────────┐
│                         procyon repo                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                       procyon CLI                           ││
│  │  ┌──────────┐    ┌──────────────┐    ┌──────────────────┐  ││
│  │  │ AST      │───▶│ IR Builder   │───▶│ Backend          │  ││
│  │  │ Parser   │    │              │    │ (Bash or Shared) │  ││
│  │  └──────────┘    └──────────────┘    └──────────────────┘  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│              ┌───────────────┴───────────────┐                  │
│              ▼                               ▼                  │
│        Bash script                    Go shared lib source      │
└─────────────────────────────────────────────────────────────────┘
```

### Shared Library Runtime

For native performance, compiled classes run as shared libraries loaded by trashtalk-daemon:

```
┌─────────────────────────────────────────────────────────────────┐
│                      trashtalk-daemon                           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  libtrashtalk (C runtime)                                   ││
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                  ││
│  │  │Counter.so│  │Person.so │  │Widget.so │  ...              ││
│  │  └──────────┘  └──────────┘  └──────────┘                  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼ Unix socket / JSON protocol       │
│                         Bash runtime                             │
└─────────────────────────────────────────────────────────────────┘
```

### Package Structure

```
procyon/
├── cmd/
│   ├── procyon/           # CLI entry point
│   ├── trashtalk-daemon/  # Shared library loader daemon
│   └── libtrashtalk/      # C runtime library
├── lib/
│   └── runtime/           # libtrashtalk headers
├── pkg/
│   ├── ast/               # Go types matching jq parser output
│   ├── parser/            # Token stream → expression tree
│   ├── ir/                # Intermediate representation
│   ├── codegen/           # Code generators (Bash, Shared)
│   ├── bytecode/          # Block VM for closure execution
│   └── runtime/           # Go runtime helpers
├── testdata/              # Acceptance test cases
└── go.mod
```

## How It Works

### 1. AST Input

The jq parser produces JSON like:

```json
{
  "type": "class",
  "name": "Counter",
  "instanceVars": [
    {"name": "value", "default": {"type": "number", "value": "0"}}
  ],
  "methods": [
    {
      "selector": "increment",
      "kind": "instance",
      "body": {"type": "block", "tokens": [...]}
    }
  ]
}
```

### 2. IR Building

The AST is converted to an intermediate representation that captures:
- Type inference for instance variables
- Method signatures and bodies
- Control flow structure

### 3. Code Generation

**Bash Backend**: Generates optimized Bash functions with proper quoting and variable handling.

**Shared Backend**: Generates Go code that:
- Exports C-compatible functions via cgo
- Links against libtrashtalk for runtime services
- Registers methods with the daemon on load

### 4. Runtime Integration

**Bash mode**: Generated scripts are sourced directly by the Trashtalk runtime.

**Shared mode**: The daemon loads dylibs on demand and dispatches method calls via JSON protocol over Unix socket.

## What Compiles

| Trashtalk | Go/Bash |
|-----------|---------|
| `instanceVars: value:0` | `type Counter struct { Value int }` / `local value=0` |
| `value` (read ivar) | `c.Value` / `$value` |
| `value := x` | `c.Value = x` / `value=$x` |
| `\| x y \|` | `var x, y int` / `local x y` |
| `x := a + b` | `x = a + b` / `x=$((a + b))` |
| `^ value` | `return value` / `echo "$value"` |
| `(a > b) ifTrue: [...]` | `if a > b { ... }` / `if ((a > b)); then ... fi` |
| `[cond] whileTrue: [...]` | `for cond { ... }` / `while ...; do ... done` |
| `@ self method` | `c.Method()` / direct call |
| `@ OtherClass method` | Shell out to Bash / message send |
| `package: MyApp` | Namespaced class support |
| `classMethod: foo [...]` | Package-level function / class function |

## What Falls Back to Bash

| Construct | Reason |
|-----------|--------|
| `rawMethod:` | Contains arbitrary Bash |
| `$(...)` subshells | Need Bash evaluation |
| Trait methods | Trait inlining not yet implemented |

## Primitive Classes

Procyon includes built-in implementations for primitive classes:

- **String**: String manipulation (trim, replace, split, etc.)
- **File**: File I/O operations
- **Shell**: Command execution and process control
- **Env**: Environment variable access
- **Console**: Terminal I/O
- **Array/Dictionary**: Collection operations
- **GrpcClient**: gRPC dynamic invocation

These are generated using the `primitiveClass:` declaration in Trashtalk.

## Testing

```bash
# Run acceptance tests
go test ./pkg/codegen/... -v

# Run all tests
go test ./...
```

### Adding Test Cases

Create a directory in `testdata/` with:
- `input.json` - AST from the jq parser
- `expected.go` - Expected generated Go code

## Roadmap

See [DESIGN.md](DESIGN.md) for the full design document.

### Completed Milestones

- **M1**: Minimal Viable Generator - struct generation, simple methods
- **M2**: Control Flow - if/else, while loops, comparisons
- **M3**: Message Sends - self/external dispatch, trait awareness
- **M4**: Namespace Support - package declarations, qualified names
- **M5**: Class Methods & Backends - Bash backend, shared library mode

### Next (M6: Polish)

- Better error messages
- `--strict` mode improvements
- Type inference improvements

## License

MIT
