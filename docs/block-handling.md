# Block Handling in the Unified Runtime

This document describes how bytecode blocks (closures) work across the Bash dispatcher, trashtalk-daemon, and native plugins.

## Architecture Overview

```
┌─────────────────┐     Unix Socket      ┌──────────────────────────────────┐
│   Bash Shell    │◄───────────────────►│       trashtalk-daemon            │
│   (trash.bash)  │    JSON protocol     │                                  │
└─────────────────┘                      │  ┌────────────────────────────┐  │
                                         │  │     GlobalRuntime          │  │
                                         │  │  ┌──────────────────────┐  │  │
                                         │  │  │    BlockRunner       │  │  │
                                         │  │  │  registry: map[id]*B │  │  │
                                         │  │  └──────────────────────┘  │  │
                                         │  │  ┌──────────────────────┐  │  │
                                         │  │  │    ObjectSpace       │  │  │
                                         │  │  └──────────────────────┘  │  │
                                         │  └────────────────────────────┘  │
                                         │                                  │
                                         │  ┌─────────┐ ┌─────────┐        │
                                         │  │ A.dylib │ │ B.dylib │ ...    │
                                         │  └─────────┘ └─────────┘        │
                                         └──────────────────────────────────┘
```

## Key Principles

### 1. Single Shared Runtime

All native plugins (`.dylib`/`.so` files) are loaded into the same process with `RTLD_GLOBAL`. They share:

- **One `GlobalRuntime`** - initialized once by `TT_Init()`
- **One `BlockRunner`** - manages all block registrations
- **One `ObjectSpace`** - stores all instances
- **One bytecode `VM`** - executes all blocks

### 2. Blocks Within Native Code

When native code works with blocks, no serialization is needed:

```
Plugin A                     BlockRunner                    Plugin B
    │                            │                              │
    │── TT_RegisterBlock() ─────►│ stores *Block               │
    │◄── returns blockID ────────│                              │
    │                            │                              │
    │── TT_Send(obj, sel, blockID)──────────────────────────────►│
    │                            │                              │
    │                            │◄── TT_InvokeBlock(blockID) ──│
    │                            │    executes via VM           │
    │                            │── returns result ───────────►│
```

Blocks are passed by ID (a string like `"bytecode_block_42"`). The receiving code looks up the block pointer in the shared `BlockRunner.registry` and executes it directly via the bytecode VM.

### 3. Blocks at the Bash Boundary

Serialization is needed when blocks cross the process boundary:

```
Bash Shell                   Daemon
    │                           │
    │── register(hex,caps) ────►│ deserialize + store
    │◄── blockID ───────────────│
    │                           │
    │   ... time passes ...     │
    │   ... daemon may restart..│
    │                           │
    │── invoke(blockID, args) ─►│ lookup + execute
    │◄── result ────────────────│
```

If the daemon restarts between registration and invocation, the block is lost. Serialization (`serialize` operation) allows Bash to retrieve the block's bytecode and re-register it with a new daemon instance.

## Block Operations

### Registration

**Request:**
```json
{
  "block_op": "register",
  "block_data": "hexencodedBytecode...",
  "block_captures": "[{\"value\": \"x\", \"name\": \"captured\"}]"
}
```

**Response:**
```json
{
  "exit_code": 0,
  "block_id": "bytecode_block_1"
}
```

### Invocation

**Request:**
```json
{
  "block_op": "invoke",
  "block_id": "bytecode_block_1",
  "block_data": "[\"arg1\", \"arg2\"]"
}
```

**Response:**
```json
{
  "exit_code": 0,
  "result": "return value"
}
```

### Serialization

**Request:**
```json
{
  "block_op": "serialize",
  "block_id": "bytecode_block_1"
}
```

**Response:**
```json
{
  "exit_code": 0,
  "block_id": "bytecode_block_1",
  "block_data": "hexencodedBytecode..."
}
```

## C API

For native plugins, blocks are managed through libtrashtalk:

```c
// Register a block (returns block ID, caller must free)
const char* TT_RegisterBlock(
    const uint8_t* bytecode, size_t len,
    TTValue** captures, int numCaptures
);

// Look up a block by ID
TTBlock* TT_LookupBlock(const char* blockID);

// Invoke by ID
TTValue TT_InvokeBlock(const char* blockID, TTValue* args, int numArgs);

// Invoke with direct pointer (faster, no lookup)
TTValue TT_InvokeBlockDirect(TTBlock* block, TTValue* args, int numArgs);
```

## When to Use What

| Scenario | Mechanism |
|----------|-----------|
| Plugin A passes block to Plugin B | Pass block ID string, B calls `TT_InvokeBlock` |
| Plugin stores block in instance var | Store block ID string |
| Native method returns block to Bash | Bash receives block ID, invokes via daemon |
| Long-running Bash process, daemon may restart | Use `serialize` to persist block data |
| Block with captured variables | Captures stored in `BlockRunner`, accessed by block ID |

## Implementation Files

- `lib/runtime/blocks.go` - `BlockRunner`, `Block`, `CaptureCell` types
- `cmd/libtrashtalk/main.go` - C API exports (`TT_RegisterBlock`, etc.)
- `cmd/trashtalk-daemon/main.go` - Socket protocol handlers
- `pkg/bytecode/chunk.go` - `Serialize()`/`Deserialize()` for bytecode
