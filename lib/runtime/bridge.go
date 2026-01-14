package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BashBridge handles fallback to Bash runtime for methods not implemented natively
type BashBridge struct {
	trashDir string // Path to ~/.trashtalk
	debug    bool
}

// NewBashBridge creates a new bash bridge
func NewBashBridge(trashDir string) *BashBridge {
	return &BashBridge{
		trashDir: trashDir,
		debug:    os.Getenv("TRASHTALK_DEBUG") != "",
	}
}

// NewBashBridgeDefault creates a bash bridge with default paths
func NewBashBridgeDefault() (*BashBridge, error) {
	trashDir := os.Getenv("TRASHTALK_ROOT")
	if trashDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}
		trashDir = filepath.Join(home, ".trashtalk")
	}
	return NewBashBridge(trashDir), nil
}

// SetDebug enables or disables debug output
func (bb *BashBridge) SetDebug(debug bool) {
	bb.debug = debug
}

// Fallback dispatches a message through the Bash runtime
// This is used when:
// - A method is marked bashOnly
// - A class isn't registered natively
// - A method isn't found in the native dispatch table
func (bb *BashBridge) Fallback(receiver string, selector string, args []Value) Value {
	trashBash := filepath.Join(bb.trashDir, "lib", "trash.bash")

	// Quote each argument to handle spaces and special characters
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = fmt.Sprintf("%q", arg.AsString())
	}

	argsStr := ""
	if len(quotedArgs) > 0 {
		argsStr = " " + strings.Join(quotedArgs, " ")
	}

	// Build the bash command
	bashCmd := fmt.Sprintf(
		"source %q && @ %s %s%s",
		trashBash,
		receiver,
		selector,
		argsStr,
	)

	if bb.debug {
		fmt.Fprintf(os.Stderr, "BashBridge: bash -c %q\n", bashCmd)
	}

	cmd := exec.Command("bash", "-c", bashCmd)
	cmd.Stderr = os.Stderr // Let errors pass through

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 200 means "method not found"
			if exitErr.ExitCode() == 200 {
				return ErrorValue(fmt.Sprintf("unknown method: %s %s", receiver, selector))
			}
			return ErrorValue(fmt.Sprintf("bash error (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr)))
		}
		return ErrorValue(fmt.Sprintf("bash failed: %v", err))
	}

	result := strings.TrimSuffix(string(output), "\n")

	if bb.debug {
		fmt.Fprintf(os.Stderr, "BashBridge: result=%q\n", result)
	}

	return StringValue(result)
}

// FallbackClassMethod dispatches a class method through Bash
func (bb *BashBridge) FallbackClassMethod(className string, selector string, args []Value) Value {
	return bb.Fallback(className, selector, args)
}

// FallbackInstanceMethod dispatches an instance method through Bash
func (bb *BashBridge) FallbackInstanceMethod(instanceID string, selector string, args []Value) Value {
	return bb.Fallback(instanceID, selector, args)
}

// InvokeBashBlock invokes a Bash-based block by its ID
func (bb *BashBridge) InvokeBashBlock(blockID string, args []Value) Value {
	trashBash := filepath.Join(bb.trashDir, "lib", "trash.bash")

	// Quote arguments
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = fmt.Sprintf("%q", arg.AsString())
	}

	argsStr := ""
	if len(quotedArgs) > 0 {
		argsStr = " " + strings.Join(quotedArgs, " ")
	}

	// Invoke block via @ blockID value args...
	bashCmd := fmt.Sprintf(
		"source %q && @ %s value%s",
		trashBash,
		blockID,
		argsStr,
	)

	if bb.debug {
		fmt.Fprintf(os.Stderr, "BashBridge: block invoke: bash -c %q\n", bashCmd)
	}

	cmd := exec.Command("bash", "-c", bashCmd)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return ErrorValue(fmt.Sprintf("block invocation failed: %v", err))
	}

	return StringValue(strings.TrimSuffix(string(output), "\n"))
}

// Eval evaluates a Trashtalk expression through the Bash runtime
func (bb *BashBridge) Eval(code string) Value {
	trashBash := filepath.Join(bb.trashDir, "lib", "trash.bash")

	bashCmd := fmt.Sprintf(
		"source %q && @ Trash eval: %q",
		trashBash,
		code,
	)

	if bb.debug {
		fmt.Fprintf(os.Stderr, "BashBridge: eval: bash -c %q\n", bashCmd)
	}

	cmd := exec.Command("bash", "-c", bashCmd)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return ErrorValue(fmt.Sprintf("eval failed: %v", err))
	}

	return StringValue(strings.TrimSuffix(string(output), "\n"))
}
