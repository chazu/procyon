<p align="center">
  <img src="https://github.com/chazu/procyon/blob/main/img/logo.png">
</p>

# Procyon

*Procyon: the genus for raccoons - because what goes better with trash?*

Procyon is a Go code generator for [Trashtalk](https://github.com/chazu/trashtalk), a Smalltalk-inspired DSL that compiles to Bash. It takes the AST output from Trashtalk's jq-based parser and generates native Go binaries that interoperate with the Bash runtime.

## Status

**M1 Complete** - Minimal viable generator working.

- Compiles simple arithmetic methods to native Go
- Falls back to Bash for unsupported constructs
- Full interop with Trashtalk runtime via shared SQLite storage

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

Procyon reads AST JSON from stdin and writes Go code to stdout:

```bash
# Generate Go code from a .trash file
./driver.bash parse Counter.trash | procyon > counter/main.go

# Copy the source file for embedding
cp Counter.trash counter/

# Build the native binary
cd counter && go build -o Counter.native .

# Install to trashtalk
cp Counter.native ~/.trashtalk/trash/.compiled/
```

### CLI Options

```
procyon [options] < ast.json > output.go

Options:
  --strict    Fail on unsupported constructs instead of warning
  --dry-run   Show what would be generated without outputting
  --version   Print version and exit
```

### Output

Procyon reports which methods were compiled and which will fall back to Bash:

```
procyon: Counter.trash
  ⚠ new - skipped: subshell expressions not supported
  ✓ getValue - compiled
  ✓ getStep - compiled
  ✓ setValue_ - compiled
  ✓ increment - compiled
  ⚠ description - skipped: class methods not yet supported

Generated 4/6 methods. 2 will fall back to Bash.
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
│  │  │ AST      │───▶│ Token Stream │───▶│ Go Code          │  ││
│  │  │ Parser   │    │ Parser       │    │ Generator        │  ││
│  │  └──────────┘    └──────────────┘    └──────────────────┘  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼                                   │
│                        main.go output                            │
└─────────────────────────────────────────────────────────────────┘
```

### Package Structure

```
procyon/
├── cmd/
│   └── procyon/
│       └── main.go           # CLI entry point
├── pkg/
│   ├── ast/
│   │   ├── types.go          # Go types matching jq parser output
│   │   └── parse.go          # JSON → AST parsing
│   ├── parser/
│   │   └── parser.go         # Token stream → expression tree
│   └── codegen/
│       ├── codegen.go        # AST → Go code (using jennifer)
│       └── codegen_test.go   # Acceptance tests
├── testdata/
│   └── counter/
│       ├── input.json        # AST from jq parser
│       └── expected.go       # Expected generated code
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

### 2. Token Stream Parsing

Method bodies come as token streams, not expression trees. The parser converts:

```
IDENTIFIER(value) PLUS NUMBER(1)
```

Into:

```go
&BinaryExpr{
  Left:  &Identifier{Name: "value"},
  Op:    "+",
  Right: &NumberLit{Value: "1"},
}
```

### 3. Code Generation

Using [jennifer](https://github.com/dave/jennifer), we generate Go code:

- Struct from `instanceVars`
- Method implementations from parsed expressions
- Dispatch switch statement
- SQLite instance storage helpers
- Embedded source and content hash

### 4. Runtime Interop

Generated binaries share the same SQLite database (`~/.trashtalk/instances.db`) as the Bash runtime. The calling convention:

```bash
# Bash calls native binary
./Counter.native <instance_id> <selector> [args...]

# Exit codes:
# 0   = success
# 200 = unknown selector (fall back to Bash)
# 1   = error
```

## What Compiles

| Trashtalk | Go |
|-----------|-----|
| `instanceVars: value:0` | `type Counter struct { Value int }` |
| `value` (read ivar) | `c.Value` |
| `value := x` (write ivar) | `c.Value = x` |
| `\| x y \|` | `var x, y int` |
| `x := a + b` | `x = a + b` |
| `^ value` | `return value` |

## What Compiles (continued)

| Trashtalk | Go |
|-----------|-----|
| `(a > b) ifTrue: [...]` | `if a > b { ... }` |
| `(a > b) ifTrue: [...] ifFalse: [...]` | `if a > b { ... } else { ... }` |
| `[cond] whileTrue: [...]` | `for cond { ... }` |
| `@ self method` | `c.Method()` (direct call) |
| `@ self keyword: arg` | `c.Keyword(arg)` (direct call) |
| `@ OtherClass method` | `sendMessage(...)` (shells out to Bash) |
| `package: MyApp` | Binary named `MyApp__Counter.native` |
| `classMethod: foo [...]` | `func Foo() string` (package-level function) |

## What Falls Back to Bash

| Construct | Reason |
|-----------|--------|
| `rawMethod:` | Contains arbitrary Bash |
| `$(...)` subshells | Need Bash evaluation |
| Trait methods | Trait inlining not yet implemented |

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

### M1: Minimal Viable Generator ✅
- Parse AST JSON
- Generate struct from instanceVars
- Generate simple arithmetic methods
- Generate dispatch switch
- Embed source and hash

### M2: Control Flow ✅
- `ifTrue:`/`ifFalse:` → `if`/`else`
- `whileTrue:` → `for` loops
- Comparison operators (`>`, `<`, `>=`, `<=`, `==`, `!=`)
- Parenthesized expressions
- Early return (`^`)

### M3: Message Sends & Traits ✅
- ✅ `@ self method` → direct method call
- ✅ `@ self keyword: arg` → method call with args
- ✅ `@ OtherClass method` → shell out to Bash via `sendMessage()`
- Trait inlining deferred (see docs/trait-inlining.md)

### M4: Namespace Support ✅
- `package:` declarations
- Qualified class names (`MyApp::Counter`)
- Correct binary naming (`MyApp__Counter.native`)
- `--info` shows package and qualified name

### M5: Class Methods ✅
- `classMethod:` compilation to package-level functions
- `dispatchClass()` for class-level dispatch
- Receiver detection (class name vs instance ID)

### M6: Polish (Next)
- Better error messages
- `--strict` mode improvements
- Type inference improvements

## License

MIT
