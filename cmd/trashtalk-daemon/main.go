// trashtalk-daemon - Dynamic plugin loader for Trashtalk
//
// This daemon loads c-shared plugins (.dylib/.so) on demand and handles
// method dispatch via Unix socket or stdin/stdout JSON protocol.
//
// Build: go build ./cmd/trashtalk-daemon
// Usage:
//   trashtalk-daemon [--plugin-dir DIR]                    # stdin/stdout mode
//   trashtalk-daemon --socket /tmp/trashtalk.sock          # socket mode
//   trashtalk-daemon --socket /tmp/trashtalk.sock --idle-timeout 300
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
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
	plugins     map[string]*Plugin // className -> plugin
	pluginDir   string
	mu          sync.RWMutex
	idleTimeout time.Duration
	idleTimer   *time.Timer
	timerMu     sync.Mutex
}

var (
	pluginDir   = flag.String("plugin-dir", "", "Directory containing .dylib/.so plugins")
	socketPath  = flag.String("socket", "", "Unix socket path (enables socket mode)")
	idleTimeout = flag.Int("idle-timeout", 300, "Idle timeout in seconds (socket mode only, 0 = no timeout)")
	debug       = flag.Bool("debug", false, "Enable debug output to stderr")
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
		plugins:     make(map[string]*Plugin),
		pluginDir:   dir,
		idleTimeout: time.Duration(*idleTimeout) * time.Second,
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: plugin-dir=%s\n", dir)
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
			fmt.Fprintf(os.Stderr, "trashtalk-daemon: request class=%s selector=%s\n", req.Class, req.Selector)
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

	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		d.respond(conn, Response{ExitCode: 1, Error: "invalid JSON: " + err.Error()})
		return
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "trashtalk-daemon: request class=%s selector=%s\n", req.Class, req.Selector)
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

	// Call plugin's Dispatch function - returns JSON with embedded exit_code
	result := d.callDispatch(plugin, req.Instance, req.Selector, string(argsJSON))

	if result == "" {
		return Response{ExitCode: 1, Error: "empty response from plugin"}
	}

	// Parse result JSON from plugin
	// The plugin returns: {"instance":{...},"result":"value","exit_code":0}
	var resultData struct {
		Instance json.RawMessage `json:"instance"`
		Result   string          `json:"result"`
		ExitCode int             `json:"exit_code"`
		Error    string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &resultData); err != nil {
		return Response{ExitCode: 1, Error: "invalid JSON from plugin: " + err.Error()}
	}

	if resultData.ExitCode == 200 {
		return Response{ExitCode: 200}
	}

	if resultData.ExitCode != 0 {
		return Response{ExitCode: resultData.ExitCode, Error: resultData.Error}
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
// The plugin returns a single JSON string with exit_code embedded to avoid struct return ABI issues
func (d *Daemon) callDispatch(plugin *Plugin, instance, selector, argsJSON string) string {
	// Convert Go strings to C strings (null-terminated)
	instancePtr := cstring(instance)
	selectorPtr := cstring(selector)
	argsPtr := cstring(argsJSON)
	defer freeStrings(instancePtr, selectorPtr, argsPtr)

	// Call Dispatch(instanceJSON, selector, argsJSON) -> *char (JSON with embedded exit_code)
	ret, _, _ := plugin.funcs.Dispatch.Call(
		uintptr(instancePtr),
		uintptr(selectorPtr),
		uintptr(argsPtr),
	)

	// The return is a single char* pointer to JSON
	return gostring(unsafe.Pointer(ret))
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
