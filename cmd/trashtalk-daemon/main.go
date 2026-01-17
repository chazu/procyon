// trashtalk-daemon - Dynamic plugin loader for Trashtalk
//
// This daemon initializes the shared runtime (libtrashtalk) and handles
// method dispatch via Unix socket or stdin/stdout JSON protocol.
//
// Build: go build ./cmd/trashtalk-daemon
// Usage:
//   trashtalk-daemon [--plugin-dir DIR]                    # stdin/stdout mode
//   trashtalk-daemon --socket /tmp/trashtalk.sock          # socket mode
//   trashtalk-daemon --socket /tmp/trashtalk.sock --idle-timeout 300
package main

/*
#cgo CFLAGS: -I${SRCDIR}/../../lib/runtime
#cgo LDFLAGS: -L${SRCDIR}/../../bin -ltrashtalk
#cgo LDFLAGS: -ldl

#include <libtrashtalk.h>
#include <stdlib.h>
#include <dlfcn.h>

// Helper to load a plugin (triggers its init() which registers with runtime)
static void* load_plugin(const char* path) {
    return dlopen(path, RTLD_NOW | RTLD_GLOBAL);
}

static const char* load_error() {
    return dlerror();
}

// Get a symbol from a loaded plugin
static void* get_symbol(void* handle, const char* name) {
    return dlsym(handle, name);
}

// Typedef for the dispatch function signature: char* ClassName_Dispatch(char* instanceJSON, char* selector, char* argsJSON)
typedef char* (*dispatch_func)(char*, char*, char*);

// Call a dispatch function pointer
static char* call_dispatch(void* fn_ptr, char* instanceJSON, char* selector, char* argsJSON) {
    dispatch_func fn = (dispatch_func)fn_ptr;
    return fn(instanceJSON, selector, argsJSON);
}
*/
import "C"

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/chazu/procyon/pkg/bytecode"
)

// Request is the JSON request from Bash
type Request struct {
	Class     string   `json:"class"`
	Instance  string   `json:"instance"`
	Selector  string   `json:"selector"`
	Args      []string `json:"args"`
	SessionID string   `json:"session_id,omitempty"` // TRASH_SESSION_ID for BashBridge fallback

	// For bytecode block operations
	BlockOp       string `json:"block_op,omitempty"`       // "register", "invoke", "serialize"
	BlockID       string `json:"block_id,omitempty"`       // Block identifier
	BlockData     string `json:"block_data,omitempty"`     // Hex-encoded bytecode or JSON args
	BlockCaptures string `json:"block_captures,omitempty"` // JSON capture cell data
}

// Response is the JSON response to Bash
type Response struct {
	Instance string `json:"instance,omitempty"`
	Result   string `json:"result,omitempty"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`

	// For bytecode block operations
	BlockID   string `json:"block_id,omitempty"`
	BlockData string `json:"block_data,omitempty"`
}

// Daemon manages plugin loading and dispatch via the shared runtime
type Daemon struct {
	pluginDir      string
	loadedPlugins  map[string]bool      // Track which plugins we've loaded
	pluginHandles  map[string]unsafe.Pointer // Store dlopen handles for dlsym
	dispatchFuncs  map[string]unsafe.Pointer // Cache dispatch function pointers
	mu             sync.RWMutex
	idleTimeout    time.Duration
	idleTimer      *time.Timer
	timerMu        sync.Mutex
}

var (
	pluginDir   = flag.String("plugin-dir", "", "Directory containing .dylib/.so plugins")
	socketPath  = flag.String("socket", "", "Unix socket path (enables socket mode)")
	idleTimeout = flag.Int("idle-timeout", 300, "Idle timeout in seconds (socket mode only, 0 = no timeout)")
	debug       = flag.Bool("debug", false, "Enable debug output to stderr")
	dbPath      = flag.String("db", "", "SQLite database path (default: ~/.trashtalk/instances.db)")
)

func main() {
	flag.Parse()

	// Determine plugin directory
	dir := *pluginDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".trashtalk", "trash", ".compiled")
	}

	// Initialize the shared runtime
	var cDbPath *C.char
	if *dbPath != "" {
		cDbPath = C.CString(*dbPath)
		defer C.free(unsafe.Pointer(cDbPath))
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: initializing shared runtime\n")
	}

	if C.TT_Init(cDbPath, nil) != 0 {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: failed to initialize shared runtime\n")
		os.Exit(1)
	}
	defer C.TT_Close()

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: shared runtime initialized\n")
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: plugin-dir=%s\n", dir)
	}

	d := &Daemon{
		pluginDir:      dir,
		loadedPlugins:  make(map[string]bool),
		pluginHandles:  make(map[string]unsafe.Pointer),
		dispatchFuncs:  make(map[string]unsafe.Pointer),
		idleTimeout:    time.Duration(*idleTimeout) * time.Second,
	}

	if *socketPath != "" {
		d.RunSocket(*socketPath)
	} else {
		d.RunStdin()
	}
}

// RunStdin processes JSON requests from stdin (original mode)
func (d *Daemon) RunStdin() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large instance JSON
	buf := make([]byte, 1024*1024) // 1MB
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			d.respond(os.Stdout, Response{ExitCode: 1, Error: "invalid JSON: " + err.Error()})
			continue
		}

		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: request class=%s selector=%s args=%v\n", req.Class, req.Selector, req.Args)
		}

		resp := d.HandleRequest(req)
		d.respond(os.Stdout, resp)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: scanner error: %v\n", err)
	}
}

// RunSocket runs the daemon in Unix socket mode
func (d *Daemon) RunSocket(path string) {
	// Remove existing socket file
	os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: failed to listen on %s: %v\n", path, err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(path)

	// Make socket world-writable so any process can connect
	os.Chmod(path, 0777)

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: listening on %s (idle-timeout=%v)\n", path, d.idleTimeout)
	}

	// Write PID file
	pidPath := path + ".pid"
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	defer os.Remove(pidPath)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: shutting down on signal\n")
		}
		listener.Close()
	}()

	// Start idle timer if timeout is set
	if d.idleTimeout > 0 {
		d.startIdleTimer(listener)
	}

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if it's because we're shutting down
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				break
			}
			if *debug {
				fmt.Fprintf(os.Stderr, "trashtalk-daemon: accept error: %v\n", err)
			}
			continue
		}

		// Reset idle timer on each connection
		d.resetIdleTimer(listener)

		// Handle connection (one request per connection)
		d.handleConnection(conn)
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: exiting\n")
	}
}

// handleConnection handles a single request on a connection
func (d *Daemon) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read deadline to prevent hanging connections
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: read error: %v\n", err)
		}
		return
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: raw request: %s\n", strings.TrimSpace(line))
	}

	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		d.respond(conn, Response{ExitCode: 1, Error: "invalid JSON: " + err.Error()})
		return
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: parsed class=%s selector=%s args=%v\n", req.Class, req.Selector, req.Args)
	}

	resp := d.HandleRequest(req)
	d.respond(conn, resp)
}

// startIdleTimer starts the idle timeout timer
func (d *Daemon) startIdleTimer(listener net.Listener) {
	d.timerMu.Lock()
	defer d.timerMu.Unlock()

	d.idleTimer = time.AfterFunc(d.idleTimeout, func() {
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: idle timeout reached, shutting down\n")
		}
		listener.Close()
	})
}

// resetIdleTimer resets the idle timeout timer
func (d *Daemon) resetIdleTimer(listener net.Listener) {
	d.timerMu.Lock()
	defer d.timerMu.Unlock()

	if d.idleTimer != nil {
		d.idleTimer.Stop()
		d.idleTimer = time.AfterFunc(d.idleTimeout, func() {
			if *debug {
				fmt.Fprintf(os.Stderr, "trashtalk-daemon: idle timeout reached, shutting down\n")
			}
			listener.Close()
		})
	}
}

func (d *Daemon) respond(w interface{ Write([]byte) (int, error) }, resp Response) {
	output, _ := json.Marshal(resp)
	w.Write(append(output, '\n'))
}

// HandleRequest processes a single dispatch request
func (d *Daemon) HandleRequest(req Request) Response {
	// Set session ID for BashBridge to ensure instances are created in the right _ENV_DIR
	if req.SessionID != "" {
		cSessionID := C.CString(req.SessionID)
		defer C.free(unsafe.Pointer(cSessionID))
		C.TT_SetSessionID(cSessionID)
	}

	// Handle bytecode block operations
	if req.BlockOp != "" {
		return d.handleBlockOp(req)
	}

	// Ensure the plugin for this class is loaded
	if err := d.ensurePluginLoaded(req.Class); err != nil {
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: no plugin for %s: %v\n", req.Class, err)
		}
		// No native plugin, signal fallback to Bash
		return Response{ExitCode: 200}
	}

	// Check if we have a dispatch function for this class
	d.mu.RLock()
	dispatchFunc := d.dispatchFuncs[req.Class]
	d.mu.RUnlock()

	if dispatchFunc != nil {
		// Call the plugin's dispatch function directly
		return d.callDispatchFunc(req, dispatchFunc)
	}

	// Fall back to TT_Send (for primitive classes that register methods directly)
	return d.callTTSend(req)
}

// callDispatchFunc calls the plugin's exported dispatch function directly
func (d *Daemon) callDispatchFunc(req Request, dispatchFunc unsafe.Pointer) Response {
	// Prepare instanceJSON - either the instance ID or the class name for class methods
	instanceJSON := req.Instance
	if instanceJSON == "" {
		instanceJSON = req.Class
	}

	// Convert args to JSON array
	argsJSON, _ := json.Marshal(req.Args)

	// Create C strings
	cInstanceJSON := C.CString(instanceJSON)
	defer C.free(unsafe.Pointer(cInstanceJSON))

	cSelector := C.CString(req.Selector)
	defer C.free(unsafe.Pointer(cSelector))

	cArgsJSON := C.CString(string(argsJSON))
	defer C.free(unsafe.Pointer(cArgsJSON))

	// Call the dispatch function via C helper: char* dispatch(char* instanceJSON, char* selector, char* argsJSON)
	cResult := C.call_dispatch(dispatchFunc, cInstanceJSON, cSelector, cArgsJSON)
	if cResult == nil {
		return Response{ExitCode: 1, Error: "dispatch returned nil"}
	}
	defer C.free(unsafe.Pointer(cResult))

	resultJSON := C.GoString(cResult)

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: dispatch result: %s\n", resultJSON)
	}

	// Parse the JSON response from the dispatch function
	// Instance can be either a string or an object (plugins return object for updated state)
	var dispatchResp struct {
		Instance json.RawMessage `json:"instance,omitempty"`
		Result   string          `json:"result,omitempty"`
		ExitCode int             `json:"exit_code"`
		Error    string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &dispatchResp); err != nil {
		return Response{ExitCode: 1, Error: "invalid dispatch response: " + err.Error()}
	}

	// Convert instance to string - it may be a JSON object or a JSON string
	instanceStr := ""
	if len(dispatchResp.Instance) > 0 {
		// Check if it's a JSON string (starts with quote) or an object
		if dispatchResp.Instance[0] == '"' {
			// It's a JSON string, unmarshal it
			json.Unmarshal(dispatchResp.Instance, &instanceStr)
		} else {
			// It's a JSON object, use as-is
			instanceStr = string(dispatchResp.Instance)
		}
	}

	return Response{
		Instance: instanceStr,
		Result:   dispatchResp.Result,
		ExitCode: dispatchResp.ExitCode,
		Error:    dispatchResp.Error,
	}
}

// callTTSend calls TT_Send through the shared runtime (for primitive classes)
func (d *Daemon) callTTSend(req Request) Response {
	// Determine the receiver - either instance ID or class name
	receiver := req.Instance
	if receiver == "" {
		receiver = req.Class
	}

	// Build arguments for TT_Send
	cReceiver := C.CString(receiver)
	defer C.free(unsafe.Pointer(cReceiver))

	cSelector := C.CString(req.Selector)
	defer C.free(unsafe.Pointer(cSelector))

	// Convert args to TTValue array
	var argsPtr *C.TTValue
	cArgs := make([]C.TTValue, len(req.Args))
	cArgStrings := make([]*C.char, len(req.Args)) // Keep references to free later

	for i, arg := range req.Args {
		cArgStrings[i] = C.CString(arg)
		cArgs[i] = C.TT_MakeString(cArgStrings[i])
	}
	defer func() {
		for _, cStr := range cArgStrings {
			C.free(unsafe.Pointer(cStr))
		}
	}()

	if len(cArgs) > 0 {
		argsPtr = &cArgs[0]
	}

	// Call TT_Send through the shared runtime
	result := C.TT_Send(cReceiver, cSelector, argsPtr, C.int(len(req.Args)))

	// Check result type for errors
	if result._type == C.TT_TYPE_ERROR {
		// Extract error message
		cErrMsg := C.TT_ValueAsString(result)
		if cErrMsg != nil {
			errMsg := C.GoString(cErrMsg)
			C.free(unsafe.Pointer(cErrMsg))
			if strings.Contains(errMsg, "unknown selector") {
				return Response{ExitCode: 200} // Signal fallback to Bash
			}
			return Response{ExitCode: 1, Error: errMsg}
		}
		return Response{ExitCode: 1, Error: "unknown error"}
	}

	// Convert result to string
	cResultStr := C.TT_ValueAsString(result)
	var resultStr string
	if cResultStr != nil {
		resultStr = C.GoString(cResultStr)
		C.free(unsafe.Pointer(cResultStr))
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: TT_Send result: %q\n", resultStr)
	}

	return Response{
		Result:   resultStr,
		ExitCode: 0,
	}
}

// ensurePluginLoaded loads the plugin for a class if not already loaded
func (d *Daemon) ensurePluginLoaded(className string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already loaded
	if d.loadedPlugins[className] {
		return nil
	}

	// Determine shared library extension
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}

	// Try to find and load the plugin
	// Handle namespaced classes: MyApp::Counter -> MyApp__Counter
	pluginName := strings.ReplaceAll(className, "::", "__")
	soPath := filepath.Join(d.pluginDir, pluginName+ext)

	if _, err := os.Stat(soPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin not found: %s", soPath)
	}

	// Load the plugin via dlopen - this triggers its init() which registers with the runtime
	cPath := C.CString(soPath)
	defer C.free(unsafe.Pointer(cPath))

	handle := C.load_plugin(cPath)
	if handle == nil {
		errMsg := C.GoString(C.load_error())
		return fmt.Errorf("failed to load %s: %s", soPath, errMsg)
	}

	// Store the handle for later dlsym calls
	d.pluginHandles[className] = handle

	// Look up the dispatch function: <ShortClassName>_Dispatch
	// For namespaced classes: Yutani::IDE -> IDE_Dispatch (uses just the class name part)
	shortClassName := className
	if idx := strings.LastIndex(className, "::"); idx != -1 {
		shortClassName = className[idx+2:]
	}
	dispatchName := shortClassName + "_Dispatch"
	cDispatchName := C.CString(dispatchName)
	defer C.free(unsafe.Pointer(cDispatchName))

	dispatchFunc := C.get_symbol(handle, cDispatchName)
	if dispatchFunc != nil {
		d.dispatchFuncs[className] = dispatchFunc
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: found dispatch func %s\n", dispatchName)
		}
	} else if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: no dispatch func %s, will use TT_Send\n", dispatchName)
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: loaded plugin %s\n", soPath)
	}

	d.loadedPlugins[className] = true
	return nil
}

// ============ Bytecode Block Operations ============

// handleBlockOp processes bytecode block operations via the shared runtime
func (d *Daemon) handleBlockOp(req Request) Response {
	switch req.BlockOp {
	case "register":
		return d.handleBlockRegister(req)
	case "invoke":
		return d.handleBlockInvoke(req)
	case "serialize":
		return d.handleBlockSerialize(req)
	default:
		return Response{ExitCode: 1, Error: "unknown block operation: " + req.BlockOp}
	}
}

// handleBlockRegister registers a new bytecode block with the shared runtime
func (d *Daemon) handleBlockRegister(req Request) Response {
	// Decode hex-encoded bytecode
	bytecodeBytes, err := hex.DecodeString(req.BlockData)
	if err != nil {
		return Response{ExitCode: 1, Error: "invalid hex bytecode: " + err.Error()}
	}

	// Deserialize bytecode into a Chunk
	chunk, err := bytecode.Deserialize(bytecodeBytes)
	if err != nil {
		return Response{ExitCode: 1, Error: "failed to deserialize bytecode: " + err.Error()}
	}

	// Parse captures JSON if provided
	var numCaptures C.int
	var capturesPtr **C.TTValue

	if req.BlockCaptures != "" && req.BlockCaptures != "[]" {
		var captures []map[string]interface{}
		if err := json.Unmarshal([]byte(req.BlockCaptures), &captures); err != nil {
			return Response{ExitCode: 1, Error: "invalid captures JSON: " + err.Error()}
		}

		if len(captures) > 0 {
			// Allocate C values for captures
			cCaptures := make([]*C.TTValue, len(captures))
			for i, cap := range captures {
				val := C.TTValue{}
				// Extract value from capture
				if v, ok := cap["value"]; ok {
					switch tv := v.(type) {
					case string:
						val._type = C.TT_TYPE_STRING
						cStr := C.CString(tv)
						*(**C.char)(unsafe.Pointer(&val.anon0)) = cStr
					case float64:
						if tv == float64(int64(tv)) {
							val._type = C.TT_TYPE_INT
							*(*C.int64_t)(unsafe.Pointer(&val.anon0)) = C.int64_t(int64(tv))
						} else {
							val._type = C.TT_TYPE_FLOAT
							*(*C.double)(unsafe.Pointer(&val.anon0)) = C.double(tv)
						}
					case bool:
						val._type = C.TT_TYPE_BOOL
						if tv {
							*(*C.int64_t)(unsafe.Pointer(&val.anon0)) = 1
						} else {
							*(*C.int64_t)(unsafe.Pointer(&val.anon0)) = 0
						}
					default:
						val._type = C.TT_TYPE_NIL
					}
				} else {
					val._type = C.TT_TYPE_NIL
				}
				cCaptures[i] = &val
			}
			numCaptures = C.int(len(captures))
			capturesPtr = &cCaptures[0]
		}
	}

	// Serialize the chunk for the C API
	serialized, err := chunk.Serialize()
	if err != nil {
		return Response{ExitCode: 1, Error: "failed to serialize chunk: " + err.Error()}
	}

	// Call TT_RegisterBlock
	cBlockID := C.TT_RegisterBlock(
		(*C.uint8_t)(unsafe.Pointer(&serialized[0])),
		C.size_t(len(serialized)),
		capturesPtr,
		numCaptures,
	)

	if cBlockID == nil {
		return Response{ExitCode: 1, Error: "TT_RegisterBlock returned nil"}
	}

	blockID := C.GoString(cBlockID)
	C.free(unsafe.Pointer(cBlockID))

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: registered block %s (%d bytes, %d captures)\n",
			blockID, len(serialized), numCaptures)
	}

	return Response{ExitCode: 0, BlockID: blockID}
}

// handleBlockInvoke invokes a registered bytecode block via TT_InvokeBlock
func (d *Daemon) handleBlockInvoke(req Request) Response {
	// Parse arguments from BlockData
	var args []string
	if req.BlockData != "" {
		if err := json.Unmarshal([]byte(req.BlockData), &args); err != nil {
			return Response{ExitCode: 1, Error: "invalid args JSON: " + err.Error()}
		}
	}

	// Convert block ID to C string
	cBlockID := C.CString(req.BlockID)
	defer C.free(unsafe.Pointer(cBlockID))

	// Build args array
	var argsPtr *C.TTValue
	cArgs := make([]C.TTValue, len(args))
	cArgStrings := make([]*C.char, len(args))

	for i, arg := range args {
		cArgStrings[i] = C.CString(arg)
		cArgs[i] = C.TT_MakeString(cArgStrings[i])
	}
	defer func() {
		for _, cStr := range cArgStrings {
			C.free(unsafe.Pointer(cStr))
		}
	}()

	if len(cArgs) > 0 {
		argsPtr = &cArgs[0]
	}

	// Call TT_InvokeBlock
	result := C.TT_InvokeBlock(cBlockID, argsPtr, C.int(len(args)))

	// Check for errors
	if result._type == C.TT_TYPE_ERROR {
		cErrMsg := C.TT_ValueAsString(result)
		if cErrMsg != nil {
			errMsg := C.GoString(cErrMsg)
			C.free(unsafe.Pointer(cErrMsg))
			// Check if block not found - signal fallback
			if strings.Contains(errMsg, "not found") {
				return Response{ExitCode: 200}
			}
			return Response{ExitCode: 1, Error: errMsg}
		}
		return Response{ExitCode: 200} // Fallback if unknown error
	}

	// Convert result to string
	cResultStr := C.TT_ValueAsString(result)
	var resultStr string
	if cResultStr != nil {
		resultStr = C.GoString(cResultStr)
		C.free(unsafe.Pointer(cResultStr))
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: invoked block %s with %d args -> %q\n",
			req.BlockID, len(args), resultStr)
	}

	return Response{ExitCode: 0, Result: resultStr}
}

// handleBlockSerialize serializes a block for cross-process transfer
func (d *Daemon) handleBlockSerialize(req Request) Response {
	if req.BlockID == "" {
		return Response{ExitCode: 1, Error: "block_id is required"}
	}

	// Look up the block via C API
	cBlockID := C.CString(req.BlockID)
	defer C.free(unsafe.Pointer(cBlockID))

	cBlock := C.TT_LookupBlock(cBlockID)
	if cBlock == nil {
		return Response{ExitCode: 1, Error: "block not found: " + req.BlockID}
	}

	// Cast to Go Block type to access fields
	// Note: This works because TTBlock is typedef'd from the Go Block struct
	type goBlock struct {
		ID         string
		Chunk      *bytecode.Chunk
		Captures   []interface{} // Actually []*CaptureCell but we just need the data
		InstanceID string
		ClassName  string
	}
	block := (*goBlock)(unsafe.Pointer(cBlock))

	if block.Chunk == nil {
		return Response{ExitCode: 1, Error: "block has no bytecode chunk"}
	}

	// Serialize the bytecode
	bytecodeBytes, err := block.Chunk.Serialize()
	if err != nil {
		return Response{ExitCode: 1, Error: "failed to serialize bytecode: " + err.Error()}
	}

	// Hex-encode the bytecode
	hexBytecode := hex.EncodeToString(bytecodeBytes)

	// Serialize captures to JSON
	// For now, we'll serialize capture values as a simple array
	capturesJSON := "[]"
	// TODO: Implement full capture serialization if needed

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: serialized block %s (%d bytes)\n",
			req.BlockID, len(bytecodeBytes))
	}

	return Response{
		ExitCode:  0,
		BlockID:   req.BlockID,
		BlockData: hexBytecode,
		Result:    capturesJSON,
	}
}
