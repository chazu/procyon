/*
 * libtrashtalk.h - Trashtalk Shared Runtime C API
 *
 * This header defines the C ABI for the Trashtalk shared runtime.
 * Plugins include this header and link against libtrashtalk.dylib.
 */

#ifndef LIBTRASHTALK_H
#define LIBTRASHTALK_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ============================================================================
 * Type Definitions
 * ============================================================================ */

/* Forward declarations */
typedef struct TTInstance TTInstance;
typedef struct TTBlock TTBlock;
typedef struct TTArray TTArray;

/* Value types */
typedef enum {
    TT_TYPE_NIL = 0,
    TT_TYPE_INT = 1,
    TT_TYPE_FLOAT = 2,
    TT_TYPE_STRING = 3,
    TT_TYPE_BOOL = 4,
    TT_TYPE_INSTANCE = 5,
    TT_TYPE_BLOCK = 6,
    TT_TYPE_ARRAY = 7,
    TT_TYPE_ERROR = 8,
} TTValueType;

/* Universal value type */
typedef struct {
    TTValueType type;
    union {
        int64_t intVal;
        double floatVal;
        const char* stringVal;
        TTInstance* instanceVal;
        TTBlock* blockVal;
        TTArray* arrayVal;
    };
} TTValue;

/* Method function signature */
typedef TTValue (*TTMethodFunc)(TTInstance* self, TTValue* args, int numArgs);

/* Method flags */
typedef enum {
    TT_METHOD_NATIVE = 1,
    TT_METHOD_BASH_FALLBACK = 2,
    TT_METHOD_CLASS_METHOD = 4,
} TTMethodFlags;

/* Method entry */
typedef struct {
    const char* selector;
    TTMethodFunc impl;
    int numArgs;
    uint32_t flags;
} TTMethodEntry;

/* Method table */
typedef struct {
    TTMethodEntry* instanceMethods;
    int numInstanceMethods;
    TTMethodEntry* classMethods;
    int numClassMethods;
} TTMethodTable;


/* ============================================================================
 * Runtime Initialization
 * ============================================================================ */

/* Initialize the global runtime.
 * dbPath: Path to instances.db (NULL for default)
 * pluginDir: Path to plugin directory (NULL for default)
 * Returns: 0 on success, -1 on error
 */
extern int TT_Init(const char* dbPath, const char* pluginDir);

/* Shut down the global runtime. */
extern void TT_Close(void);


/* ============================================================================
 * Class Registration
 * ============================================================================ */

/* Register a class with the runtime.
 * Called from plugin init() at load time.
 */
extern void TT_RegisterClass(
    const char* className,
    const char* superclass,
    const char** instanceVars,
    int numVars,
    TTMethodTable* methods
);


/* ============================================================================
 * Instance Creation and Lookup
 * ============================================================================ */

/* Create a new instance of a class.
 * Returns: Instance ID (caller must free)
 */
extern const char* TT_New(const char* className);

/* Look up an instance by ID.
 * Returns: Instance pointer or NULL
 */
extern TTInstance* TT_Lookup(const char* instanceID);


/* ============================================================================
 * Instance Variable Access
 * ============================================================================ */

/* Get instance variable by name */
extern TTValue TT_GetIVar(TTInstance* inst, const char* name);

/* Set instance variable by name */
extern void TT_SetIVar(TTInstance* inst, const char* name, TTValue val);

/* Get instance variable by index (faster) */
extern TTValue TT_GetIVarByIndex(TTInstance* inst, int index);

/* Set instance variable by index (faster) */
extern void TT_SetIVarByIndex(TTInstance* inst, int index, TTValue val);


/* ============================================================================
 * Message Dispatch
 * ============================================================================ */

/* Send a message to a receiver (by ID or class name).
 * receiver: Instance ID or class name
 * selector: Method name
 * args: Array of argument values
 * numArgs: Number of arguments
 * Returns: Result value
 */
extern TTValue TT_Send(
    const char* receiver,
    const char* selector,
    TTValue* args,
    int numArgs
);

/* Send a message with a direct instance pointer.
 * Faster than TT_Send when you already have the pointer.
 */
extern TTValue TT_SendDirect(
    TTInstance* receiver,
    const char* selector,
    TTValue* args,
    int numArgs
);


/* ============================================================================
 * Block Operations
 * ============================================================================ */

/* Register a bytecode block.
 * Returns: Block ID (caller must free)
 */
extern const char* TT_RegisterBlock(
    const uint8_t* bytecode,
    size_t bytecodeLen,
    TTValue** captures,
    int numCaptures
);

/* Look up a block by ID.
 * Returns: Block pointer or NULL
 */
extern TTBlock* TT_LookupBlock(const char* blockID);

/* Invoke a block by ID. */
extern TTValue TT_InvokeBlock(
    const char* blockID,
    TTValue* args,
    int numArgs
);

/* Invoke a block with direct pointer. */
extern TTValue TT_InvokeBlockDirect(
    TTBlock* block,
    TTValue* args,
    int numArgs
);


/* ============================================================================
 * Persistence
 * ============================================================================ */

/* Save an instance to the database.
 * Returns: 0 on success, -1 on error
 */
extern int TT_Persist(const char* instanceID);

/* Load an instance from the database.
 * Returns: Instance pointer or NULL
 */
extern TTInstance* TT_Load(const char* instanceID);


/* ============================================================================
 * Bash Bridge
 * ============================================================================ */

/* Set the session ID for BashBridge.
 * This ensures BashBridge uses the same _ENV_DIR as the client.
 * Call this before each request that may use bash fallback.
 */
extern void TT_SetSessionID(const char* sessionID);

/* Fall back to Bash runtime for a message.
 * Used for bashOnly methods or unregistered classes.
 * Returns: Result string (caller must free)
 */
extern const char* TT_BashFallback(
    const char* receiver,
    const char* selector,
    const char** args,
    int numArgs
);


/* ============================================================================
 * Serialization (Bash Boundary)
 * ============================================================================ */

/* Serialize an instance to JSON.
 * Used when passing objects to Bash.
 * Returns: JSON string (caller must free)
 */
extern const char* TT_Serialize(TTInstance* inst);

/* Deserialize JSON to an instance.
 * Used when receiving objects from Bash.
 * Returns: Instance pointer or NULL
 */
extern TTInstance* TT_Deserialize(const char* jsonStr);

/* Get instance ID */
extern const char* TT_GetInstanceID(TTInstance* inst);

/* Get instance class name */
extern const char* TT_GetInstanceClass(TTInstance* inst);


/* ============================================================================
 * Value Helpers
 * ============================================================================ */

/* Create values */
extern TTValue TT_MakeNil(void);
extern TTValue TT_MakeInt(int64_t n);
extern TTValue TT_MakeFloat(double f);
extern TTValue TT_MakeString(const char* s);
extern TTValue TT_MakeBool(int b);
extern TTValue TT_MakeInstance(TTInstance* inst);

/* Extract values */
extern const char* TT_ValueAsString(TTValue val);
extern int64_t TT_ValueAsInt(TTValue val);
extern const char* TT_ValueToJSON(TTValue val);

/* Error handling */
extern const char* TT_GetLastError(void);


/* ============================================================================
 * Macros for plugin development
 * ============================================================================ */

/* Helper to define an instance method */
#define TT_INSTANCE_METHOD(name, numArgs) \
    TTValue name(TTInstance* self, TTValue* args, int _numArgs)

/* Helper to define a class method */
#define TT_CLASS_METHOD(name, numArgs) \
    TTValue name(TTInstance* self, TTValue* args, int _numArgs)

/* Helper to build method entry */
#define TT_METHOD_ENTRY(sel, fn, argc, flags) \
    { .selector = sel, .impl = fn, .numArgs = argc, .flags = flags }

/* Helper to get argument */
#define TT_ARG(n) (args[n])
#define TT_ARG_STRING(n) TT_ValueAsString(args[n])
#define TT_ARG_INT(n) TT_ValueAsInt(args[n])

/* Helper to get/set ivars */
#define TT_IVAR(inst, name) TT_GetIVar(inst, name)
#define TT_SET_IVAR(inst, name, val) TT_SetIVar(inst, name, val)


#ifdef __cplusplus
}
#endif

#endif /* LIBTRASHTALK_H */
