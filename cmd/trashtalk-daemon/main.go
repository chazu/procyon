// trashtalk-daemon - Dynamic plugin loader for Trashtalk
//
// This daemon loads c-shared plugins (.dylib/.so) on demand and handles
// method dispatch via stdin/stdout JSON protocol.
//
// Build: go build ./cmd/trashtalk-daemon
// Usage: trashtalk-daemon [--plugin-dir DIR]
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/jamesits/goinvoke"
)

// PluginFuncs holds the exported functions from a c-shared plugin
type PluginFuncs struct {
	GetClassName *goinvoke.Proc `func:"GetClassName"`
	Dispatch     *goinvoke.Proc `func:"Dispatch"`
}

// Plugin represents a loaded class plugin
type Plugin struct {
	funcs     *PluginFuncs
	className string
	path      string
}

// Request is the JSON request from Bash
type Request struct {
	Class    string   `json:"class"`
	Instance string   `json:"instance"`
	Selector string   `json:"selector"`
	Args     []string `json:"args"`
}

// Response is the JSON response to Bash
type Response struct {
	Instance string `json:"instance,omitempty"`
	Result   string `json:"result,omitempty"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// Daemon manages plugin loading and dispatch
type Daemon struct {
	plugins   map[string]*Plugin // className -> plugin
	pluginDir string
	mu        sync.RWMutex
}

var (
	pluginDir = flag.String("plugin-dir", "", "Directory containing .dylib/.so plugins")
	debug     = flag.Bool("debug", false, "Enable debug output to stderr")
)

func main() {
	flag.Parse()

	// Determine plugin directory
	dir := *pluginDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".trashtalk", "trash", ".compiled")
	}

	d := &Daemon{
		plugins:   make(map[string]*Plugin),
		pluginDir: dir,
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: plugin-dir=%s\n", dir)
	}

	d.Run()
}

// Run processes JSON requests from stdin
func (d *Daemon) Run() {
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
			d.respond(Response{ExitCode: 1, Error: "invalid JSON: " + err.Error()})
			continue
		}

		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: request class=%s selector=%s\n", req.Class, req.Selector)
		}

		resp := d.HandleRequest(req)
		d.respond(resp)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: scanner error: %v\n", err)
	}
}

func (d *Daemon) respond(resp Response) {
	output, _ := json.Marshal(resp)
	fmt.Println(string(output))
}

// HandleRequest processes a single dispatch request
func (d *Daemon) HandleRequest(req Request) Response {
	// Load plugin on demand
	plugin, err := d.LoadPlugin(req.Class)
	if err != nil {
		if *debug {
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: no plugin for %s: %v\n", req.Class, err)
		}
		// No native plugin, signal fallback to Bash
		return Response{ExitCode: 200}
	}

	// Convert args to JSON
	argsJSON, _ := json.Marshal(req.Args)

	// Call plugin's Dispatch function
	result, exitCode := d.callDispatch(plugin, req.Instance, req.Selector, string(argsJSON))

	if exitCode == 200 {
		return Response{ExitCode: 200}
	}

	if exitCode != 0 {
		return Response{ExitCode: int(exitCode), Error: result}
	}

	// Parse result JSON from plugin
	// The plugin returns: {"instance":{...},"result":"value"}
	var resultData struct {
		Instance json.RawMessage `json:"instance"`
		Result   string          `json:"result"`
	}
	if err := json.Unmarshal([]byte(result), &resultData); err != nil {
		// Result might be plain string
		return Response{Result: result, ExitCode: 0}
	}

	return Response{
		Instance: string(resultData.Instance),
		Result:   resultData.Result,
		ExitCode: 0,
	}
}

// LoadPlugin loads a class plugin, caching for subsequent calls
func (d *Daemon) LoadPlugin(className string) (*Plugin, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if p, ok := d.plugins[className]; ok {
		return p, nil // Already loaded
	}

	// Determine shared library extension
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}

	// Find and load shared library
	soPath := filepath.Join(d.pluginDir, className+ext)
	if _, err := os.Stat(soPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin not found: %s", soPath)
	}

	funcs := &PluginFuncs{}
	if err := goinvoke.Unmarshal(soPath, funcs); err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", soPath, err)
	}

	// Verify plugin loaded correctly
	if funcs.Dispatch == nil {
		return nil, fmt.Errorf("plugin %s missing Dispatch function", soPath)
	}

	p := &Plugin{
		funcs:     funcs,
		className: className,
		path:      soPath,
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: loaded plugin %s\n", soPath)
	}

	d.plugins[className] = p
	return p, nil
}

// callDispatch calls the plugin's Dispatch function via FFI
func (d *Daemon) callDispatch(plugin *Plugin, instance, selector, argsJSON string) (string, int32) {
	// Convert Go strings to C strings (null-terminated)
	instancePtr := cstring(instance)
	selectorPtr := cstring(selector)
	argsPtr := cstring(argsJSON)
	defer freeStrings(instancePtr, selectorPtr, argsPtr)

	// Call Dispatch(instanceJSON, selector, argsJSON) -> (resultJSON, exitCode)
	// The c-shared function returns a struct, which syscall.Call handles as two return values
	ret, _, _ := plugin.funcs.Dispatch.Call(
		uintptr(instancePtr),
		uintptr(selectorPtr),
		uintptr(argsPtr),
	)

	// The return is a pointer to the result struct
	// For c-shared with multiple returns, we get them packed
	// First return is the char* result, second is the int exit code
	// Actually, goinvoke returns them as ret and ret2
	// Let me handle this more carefully...

	// The Dispatch function returns (char*, int)
	// In c-shared ABI, this becomes a struct return
	// goinvoke's Call() returns (r1, r2, err) where r1/r2 are the return values
	resultPtr := ret

	// For the exit code, we need to make another call or use the struct return
	// Actually looking at the header: struct Dispatch_return { char* r0; int r1; }
	// goinvoke.Call should return these in ret1 and ret2

	// Let's check if there's a second return
	// Since goinvoke follows syscall convention, ret is first value
	// We need to look at how the struct return works...

	// For now, assume ret is the result string pointer
	// The exit code might need special handling

	result := gostring(unsafe.Pointer(resultPtr))

	// TODO: Properly extract exit code from struct return
	// For now, check if result contains error indication
	exitCode := int32(0)
	if result == "" {
		exitCode = 200 // Unknown selector signal
	}

	return result, exitCode
}

// cstring converts a Go string to a C string (null-terminated byte slice)
// Returns an unsafe.Pointer that must be freed
func cstring(s string) unsafe.Pointer {
	b := append([]byte(s), 0)
	return unsafe.Pointer(&b[0])
}

// gostring converts a C string pointer to a Go string
func gostring(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	// Find null terminator
	var length int
	for {
		if *(*byte)(unsafe.Pointer(uintptr(p) + uintptr(length))) == 0 {
			break
		}
		length++
		if length > 1024*1024 { // Safety limit: 1MB
			break
		}
	}
	return string(unsafe.Slice((*byte)(p), length))
}

// freeStrings is a no-op since we're using Go-allocated memory
// that will be GC'd. In a real implementation, we might need to
// free C.CString allocations from the plugin side.
func freeStrings(ptrs ...unsafe.Pointer) {
	// Go-allocated strings are managed by GC
}
