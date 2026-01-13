package main

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/chazu/procyon/pkg/bytecode"
)

// TestDaemonBlockRegistry tests block registration and retrieval
func TestDaemonBlockRegistry(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create a simple chunk that returns "42"
	chunk := bytecode.NewChunk()
	idx := chunk.AddConstant("42")
	chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
	chunk.Emit(bytecode.OpReturn)

	// Register the block
	blockID := d.registerBlock(chunk, nil)

	if blockID == "" {
		t.Fatal("Expected non-empty block ID")
	}

	if !isValidBlockID(blockID) {
		t.Errorf("Invalid block ID format: %s", blockID)
	}

	// Verify block is in registry
	d.blockMu.RLock()
	block, ok := d.blockRegistry[blockID]
	d.blockMu.RUnlock()

	if !ok {
		t.Fatalf("Block %s not found in registry", blockID)
	}

	if block.Chunk == nil {
		t.Error("Registered block has nil chunk")
	}
}

// TestDaemonBlockRegisterRequest tests the register block operation via Request
func TestDaemonBlockRegisterRequest(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create and serialize a chunk
	chunk := bytecode.NewChunk()
	idx := chunk.AddConstant("hello")
	chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
	chunk.Emit(bytecode.OpReturn)

	data, err := chunk.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize chunk: %v", err)
	}

	// Create register request
	req := Request{
		BlockOp:   "register",
		BlockData: hex.EncodeToString(data),
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode != 0 {
		t.Fatalf("Register failed: exit_code=%d, error=%s", resp.ExitCode, resp.Error)
	}

	if resp.BlockID == "" {
		t.Error("Expected block_id in response")
	}

	if !isValidBlockID(resp.BlockID) {
		t.Errorf("Invalid block ID: %s", resp.BlockID)
	}
}

// TestDaemonBlockInvokeRequest tests invoking a registered block
func TestDaemonBlockInvokeRequest(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create a chunk that returns "success"
	chunk := bytecode.NewChunk()
	idx := chunk.AddConstant("success")
	chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
	chunk.Emit(bytecode.OpReturn)

	// Register it
	blockID := d.registerBlock(chunk, nil)

	// Invoke request
	req := Request{
		BlockOp:   "invoke",
		BlockID:   blockID,
		BlockData: "[]", // Empty args
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode != 0 {
		t.Fatalf("Invoke failed: exit_code=%d, error=%s", resp.ExitCode, resp.Error)
	}

	if resp.Result != "success" {
		t.Errorf("Expected result 'success', got %q", resp.Result)
	}
}

// TestDaemonBlockInvokeWithArgs tests invoking a block with arguments
func TestDaemonBlockInvokeWithArgs(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create a chunk that adds two parameters: [:x :y | x + y]
	chunk := bytecode.NewChunk()
	chunk.ParamCount = 2
	chunk.ParamNames = []string{"x", "y"}

	// Load param 0, load param 1, add, return
	chunk.EmitWithOperand(bytecode.OpLoadParam, 0)
	chunk.EmitWithOperand(bytecode.OpLoadParam, 1)
	chunk.Emit(bytecode.OpAdd)
	chunk.Emit(bytecode.OpReturn)

	blockID := d.registerBlock(chunk, nil)

	// Invoke with args "10" and "32"
	argsJSON, _ := json.Marshal([]string{"10", "32"})
	req := Request{
		BlockOp:   "invoke",
		BlockID:   blockID,
		BlockData: string(argsJSON),
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode != 0 {
		t.Fatalf("Invoke failed: exit_code=%d, error=%s", resp.ExitCode, resp.Error)
	}

	if resp.Result != "42" {
		t.Errorf("Expected result '42', got %q", resp.Result)
	}
}

// TestDaemonBlockInvokeNotFound tests invoking a non-existent block
func TestDaemonBlockInvokeNotFound(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Try to invoke a non-existent bytecode block
	req := Request{
		BlockOp:   "invoke",
		BlockID:   "bytecode_block_99999",
		BlockData: "[]",
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode == 0 {
		t.Error("Expected error for non-existent block")
	}
}

// TestDaemonBlockInvokeFallback tests that non-bytecode blocks return 200
func TestDaemonBlockInvokeFallback(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Try to invoke a bash block (should fall back)
	req := Request{
		BlockOp:   "invoke",
		BlockID:   "bash_block_12345", // Not a bytecode block
		BlockData: "[]",
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode != 200 {
		t.Errorf("Expected exit_code 200 for bash block fallback, got %d", resp.ExitCode)
	}
}

// TestDaemonBlockSerialize tests serializing a registered block
func TestDaemonBlockSerialize(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create and register a chunk
	chunk := bytecode.NewChunk()
	idx := chunk.AddConstant("test")
	chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
	chunk.Emit(bytecode.OpReturn)

	blockID := d.registerBlock(chunk, nil)

	// Serialize request
	req := Request{
		BlockOp: "serialize",
		BlockID: blockID,
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode != 0 {
		t.Fatalf("Serialize failed: exit_code=%d, error=%s", resp.ExitCode, resp.Error)
	}

	if resp.BlockData == "" {
		t.Error("Expected block_data in response")
	}

	// Verify we can deserialize the data
	data, err := hex.DecodeString(resp.BlockData)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	_, err = bytecode.Deserialize(data)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}
}

// TestDaemonBlockWithCaptures tests block registration with captures
func TestDaemonBlockWithCaptures(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create a chunk that uses a capture
	chunk := bytecode.NewChunk()
	chunk.CaptureInfo = []bytecode.CaptureDescriptor{
		{Name: "x", Source: bytecode.VarSourceLocal},
	}
	chunk.EmitWithOperand(bytecode.OpLoadCapture, 0)
	chunk.Emit(bytecode.OpReturn)

	data, _ := chunk.Serialize()

	// Create capture data JSON
	captures := []struct {
		Value      string `json:"value"`
		Name       string `json:"name"`
		Source     int    `json:"source"`
		InstanceID string `json:"instance_id"`
	}{
		{Value: "captured_value", Name: "x", Source: 0},
	}
	capturesJSON, _ := json.Marshal(captures)

	// Register with captures
	req := Request{
		BlockOp:       "register",
		BlockData:     hex.EncodeToString(data),
		BlockCaptures: string(capturesJSON),
	}

	resp := d.HandleRequest(req)
	if resp.ExitCode != 0 {
		t.Fatalf("Register failed: %s", resp.Error)
	}

	// Invoke and verify capture value is used
	invokeReq := Request{
		BlockOp:   "invoke",
		BlockID:   resp.BlockID,
		BlockData: "[]",
	}

	invokeResp := d.HandleRequest(invokeReq)
	if invokeResp.ExitCode != 0 {
		t.Fatalf("Invoke failed: %s", invokeResp.Error)
	}

	if invokeResp.Result != "captured_value" {
		t.Errorf("Expected 'captured_value', got %q", invokeResp.Result)
	}
}

// TestDaemonBlockUnknownOp tests handling of unknown block operations
func TestDaemonBlockUnknownOp(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	req := Request{
		BlockOp: "unknown_operation",
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode == 0 {
		t.Error("Expected error for unknown operation")
	}

	if resp.Error == "" {
		t.Error("Expected error message")
	}
}

// TestDaemonBlockInvalidBytecode tests handling of invalid bytecode
func TestDaemonBlockInvalidBytecode(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	req := Request{
		BlockOp:   "register",
		BlockData: "not_valid_hex",
	}

	resp := d.HandleRequest(req)

	if resp.ExitCode == 0 {
		t.Error("Expected error for invalid hex")
	}
}

// TestDaemonMultipleBlocks tests registering and invoking multiple blocks
func TestDaemonMultipleBlocks(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Register 3 blocks with different return values
	blockIDs := make([]string, 3)
	values := []string{"one", "two", "three"}

	for i, val := range values {
		chunk := bytecode.NewChunk()
		idx := chunk.AddConstant(val)
		chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
		chunk.Emit(bytecode.OpReturn)
		blockIDs[i] = d.registerBlock(chunk, nil)
	}

	// Verify each returns correct value
	for i, blockID := range blockIDs {
		req := Request{
			BlockOp:   "invoke",
			BlockID:   blockID,
			BlockData: "[]",
		}

		resp := d.HandleRequest(req)
		if resp.ExitCode != 0 {
			t.Errorf("Block %d invoke failed: %s", i, resp.Error)
			continue
		}

		if resp.Result != values[i] {
			t.Errorf("Block %d: expected %q, got %q", i, values[i], resp.Result)
		}
	}
}

// ============ Stress Tests ============

// TestDaemonBlockConcurrentInvoke tests concurrent block invocations
func TestDaemonBlockConcurrentInvoke(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	// Create a block that adds its param to 100
	chunk := bytecode.NewChunk()
	chunk.ParamCount = 1
	chunk.ParamNames = []string{"n"}
	idx := chunk.AddConstant("100")
	chunk.EmitWithOperand(bytecode.OpLoadParam, 0)
	chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
	chunk.Emit(bytecode.OpAdd)
	chunk.Emit(bytecode.OpReturn)

	blockID := d.registerBlock(chunk, nil)

	// Run 100 concurrent invocations
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)
	errors := make(chan string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			argsJSON, _ := json.Marshal([]string{string(rune('0' + n%10))})
			req := Request{
				BlockOp:   "invoke",
				BlockID:   blockID,
				BlockData: string(argsJSON),
			}

			resp := d.HandleRequest(req)
			if resp.ExitCode != 0 {
				errors <- resp.Error
			}
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent invocation error: %s", err)
	}
}

// TestDaemonBlockConcurrentRegister tests concurrent block registrations
func TestDaemonBlockConcurrentRegister(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	const numGoroutines = 50
	blockIDs := make(chan string, numGoroutines)

	// Register blocks concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			chunk := bytecode.NewChunk()
			val := string(rune('a' + n%26))
			idx := chunk.AddConstant(val)
			chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
			chunk.Emit(bytecode.OpReturn)

			data, _ := chunk.Serialize()
			req := Request{
				BlockOp:   "register",
				BlockData: hex.EncodeToString(data),
			}

			resp := d.HandleRequest(req)
			if resp.ExitCode == 0 {
				blockIDs <- resp.BlockID
			} else {
				blockIDs <- ""
			}
		}(i)
	}

	// Collect results
	uniqueIDs := make(map[string]bool)
	for i := 0; i < numGoroutines; i++ {
		id := <-blockIDs
		if id == "" {
			t.Error("Registration failed")
			continue
		}
		if uniqueIDs[id] {
			t.Errorf("Duplicate block ID: %s", id)
		}
		uniqueIDs[id] = true
	}

	// Should have all unique IDs
	if len(uniqueIDs) != numGoroutines {
		t.Errorf("Expected %d unique IDs, got %d", numGoroutines, len(uniqueIDs))
	}
}

// TestDaemonBlockManyBlocks tests registering many blocks
func TestDaemonBlockManyBlocks(t *testing.T) {
	d := &Daemon{
		plugins:       make(map[string]*Plugin),
		blockRegistry: make(map[string]*RegisteredBlock),
		vm:            bytecode.NewVM(),
	}

	const numBlocks = 1000

	// Register many blocks
	blockIDs := make([]string, numBlocks)
	for i := 0; i < numBlocks; i++ {
		chunk := bytecode.NewChunk()
		val := string(rune(i % 256))
		idx := chunk.AddConstant(val)
		chunk.EmitWithOperand(bytecode.OpConst, uint16Bytes(idx)...)
		chunk.Emit(bytecode.OpReturn)

		blockIDs[i] = d.registerBlock(chunk, nil)
	}

	// Verify registry size
	d.blockMu.RLock()
	registrySize := len(d.blockRegistry)
	d.blockMu.RUnlock()

	if registrySize != numBlocks {
		t.Errorf("Expected %d blocks in registry, got %d", numBlocks, registrySize)
	}

	// Invoke a few to verify they still work
	for i := 0; i < 10; i++ {
		idx := i * (numBlocks / 10)
		req := Request{
			BlockOp:   "invoke",
			BlockID:   blockIDs[idx],
			BlockData: "[]",
		}

		resp := d.HandleRequest(req)
		if resp.ExitCode != 0 {
			t.Errorf("Failed to invoke block %d: %s", idx, resp.Error)
		}
	}
}

// Helper functions

func isValidBlockID(id string) bool {
	return len(id) > 0 && id[:15] == "bytecode_block_"
}

func uint16Bytes(v uint16) []byte {
	return []byte{byte(v >> 8), byte(v)}
}
