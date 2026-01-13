package bytecode

import (
	"encoding/binary"
	"testing"
)

// MockMessageSender implements MessageSender for testing.
type MockMessageSender struct {
	Calls []MockCall
}

type MockCall struct {
	Receiver string
	Selector string
	Args     []string
	Result   string
	Err      error
}

func (m *MockMessageSender) SendMessage(receiver, selector string, args ...string) (string, error) {
	for _, call := range m.Calls {
		if call.Receiver == receiver && call.Selector == selector {
			return call.Result, call.Err
		}
	}
	return "", nil
}

// MockInstanceAccessor implements InstanceAccessor for testing.
type MockInstanceAccessor struct {
	Vars map[string]map[string]string // instanceID -> varName -> value
}

func NewMockInstanceAccessor() *MockInstanceAccessor {
	return &MockInstanceAccessor{
		Vars: make(map[string]map[string]string),
	}
}

func (m *MockInstanceAccessor) GetInstanceVar(instanceID, varName string) (string, error) {
	if vars, ok := m.Vars[instanceID]; ok {
		if val, ok := vars[varName]; ok {
			return val, nil
		}
	}
	return "", nil
}

func (m *MockInstanceAccessor) SetInstanceVar(instanceID, varName, value string) error {
	if m.Vars[instanceID] == nil {
		m.Vars[instanceID] = make(map[string]string)
	}
	m.Vars[instanceID][varName] = value
	return nil
}

// Helper to create a chunk with code
func chunkWithCode(code ...byte) *Chunk {
	c := NewChunk()
	c.Code = code
	return c
}

// Helper to emit uint16 as big-endian bytes
func uint16Bytes(v uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return buf
}

// ============ Stack Operation Tests ============

func TestVMStackPushPop(t *testing.T) {
	// Push constant, then return it
	chunk := NewChunk()
	idx := chunk.AddConstant("hello")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

func TestVMNop(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpNop)
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
}

func TestVMPop(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpConstFalse)
	chunk.Emit(OpPop) // Pop false
	chunk.Emit(OpReturn) // Return true

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
}

func TestVMDup(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstOne)
	chunk.Emit(OpDup)
	chunk.Emit(OpAdd)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "2" {
		t.Errorf("Expected '2', got %q", result)
	}
}

func TestVMSwap(t *testing.T) {
	chunk := NewChunk()
	idx1 := chunk.AddConstant("10")
	idx2 := chunk.AddConstant("3")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx1)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx2)...)
	chunk.Emit(OpSwap)
	chunk.Emit(OpSub) // 3 - 10 = -7
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "-7" {
		t.Errorf("Expected '-7', got %q", result)
	}
}

func TestVMRot(t *testing.T) {
	chunk := NewChunk()
	idx1 := chunk.AddConstant("1")
	idx2 := chunk.AddConstant("2")
	idx3 := chunk.AddConstant("3")
	// Push 1, 2, 3 -> stack: [1, 2, 3]
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx1)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx2)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx3)...)
	// Rotate: [1, 2, 3] -> [2, 3, 1]
	chunk.Emit(OpRot)
	chunk.Emit(OpReturn) // Return 1

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "1" {
		t.Errorf("Expected '1', got %q", result)
	}
}

// ============ Constant Tests ============

func TestVMConstNil(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstNil), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestVMConstTrue(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstTrue), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
}

func TestVMConstFalse(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstFalse), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "false" {
		t.Errorf("Expected 'false', got %q", result)
	}
}

func TestVMConstZero(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstZero), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "0" {
		t.Errorf("Expected '0', got %q", result)
	}
}

func TestVMConstOne(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstOne), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "1" {
		t.Errorf("Expected '1', got %q", result)
	}
}

func TestVMConstEmpty(t *testing.T) {
	chunk := chunkWithCode(byte(OpConstEmpty), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

// ============ Local Variable Tests ============

func TestVMLoadStoreLocal(t *testing.T) {
	chunk := NewChunk()
	chunk.LocalCount = 2

	idx := chunk.AddConstant("42")
	// Store 42 to slot 0
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.EmitWithOperand(OpStoreLocal, 0)

	// Store "hello" to slot 1
	idx2 := chunk.AddConstant("hello")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx2)...)
	chunk.EmitWithOperand(OpStoreLocal, 1)

	// Load slot 0 and return
	chunk.EmitWithOperand(OpLoadLocal, 0)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "42" {
		t.Errorf("Expected '42', got %q", result)
	}
}

func TestVMLoadParam(t *testing.T) {
	chunk := NewChunk()
	chunk.ParamNames = []string{"x", "y"}

	// Load param 0
	chunk.EmitWithOperand(OpLoadParam, 0)
	// Load param 1
	chunk.EmitWithOperand(OpLoadParam, 1)
	// Add them
	chunk.Emit(OpAdd)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", []string{"10", "5"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "15" {
		t.Errorf("Expected '15', got %q", result)
	}
}

func TestVMLoadMissingParam(t *testing.T) {
	chunk := NewChunk()
	chunk.ParamNames = []string{"x"}

	// Load param 5 (doesn't exist)
	chunk.EmitWithOperand(OpLoadParam, 5)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", []string{"10"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string for missing param, got %q", result)
	}
}

// ============ Capture Tests ============

func TestVMLoadCapture(t *testing.T) {
	chunk := NewChunk()
	chunk.CaptureInfo = []CaptureDescriptor{
		{Name: "x", Source: VarSourceLocal},
	}
	chunk.EmitWithOperand(OpLoadCapture, 0)
	chunk.Emit(OpReturn)

	vm := NewVM()
	captures := []*CaptureCell{
		{Value: "captured_value", Source: VarSourceLocal},
	}
	result, err := vm.ExecuteWithCaptures(chunk, "", nil, captures)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "captured_value" {
		t.Errorf("Expected 'captured_value', got %q", result)
	}
}

func TestVMStoreCapture(t *testing.T) {
	chunk := NewChunk()
	chunk.CaptureInfo = []CaptureDescriptor{
		{Name: "x", Source: VarSourceLocal},
	}

	idx := chunk.AddConstant("new_value")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.EmitWithOperand(OpStoreCapture, 0)
	chunk.EmitWithOperand(OpLoadCapture, 0)
	chunk.Emit(OpReturn)

	vm := NewVM()
	captures := []*CaptureCell{
		{Value: "old_value", Source: VarSourceLocal},
	}
	result, err := vm.ExecuteWithCaptures(chunk, "", nil, captures)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "new_value" {
		t.Errorf("Expected 'new_value', got %q", result)
	}
	if captures[0].Value != "new_value" {
		t.Errorf("Capture not updated: expected 'new_value', got %q", captures[0].Value)
	}
}

// ============ Instance Variable Tests ============

func TestVMLoadStoreIVar(t *testing.T) {
	chunk := NewChunk()
	nameIdx := chunk.AddConstant("count")

	// Store "42" to @count
	valIdx := chunk.AddConstant("42")
	chunk.EmitWithOperand(OpConst, uint16Bytes(valIdx)...)
	chunk.EmitWithOperand(OpStoreIVar, uint16Bytes(nameIdx)...)

	// Load @count
	chunk.EmitWithOperand(OpLoadIVar, uint16Bytes(nameIdx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	accessor := NewMockInstanceAccessor()
	vm.SetInstanceAccessor(accessor)

	result, err := vm.Execute(chunk, "counter_123", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "42" {
		t.Errorf("Expected '42', got %q", result)
	}
}

// ============ Arithmetic Tests ============

func TestVMAdd(t *testing.T) {
	tests := []struct {
		a, b   string
		expect string
	}{
		{"5", "3", "8"},
		{"0", "0", "0"},
		{"-5", "3", "-2"},
		{"100", "200", "300"},
	}

	for _, tt := range tests {
		chunk := NewChunk()
		idxA := chunk.AddConstant(tt.a)
		idxB := chunk.AddConstant(tt.b)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
		chunk.Emit(OpAdd)
		chunk.Emit(OpReturn)

		vm := NewVM()
		result, err := vm.Execute(chunk, "", nil)
		if err != nil {
			t.Fatalf("Execute failed for %s + %s: %v", tt.a, tt.b, err)
		}
		if result != tt.expect {
			t.Errorf("%s + %s = %q, want %q", tt.a, tt.b, result, tt.expect)
		}
	}
}

func TestVMSub(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("10")
	idxB := chunk.AddConstant("3")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
	chunk.Emit(OpSub)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "7" {
		t.Errorf("10 - 3 = %q, want '7'", result)
	}
}

func TestVMMul(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("6")
	idxB := chunk.AddConstant("7")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
	chunk.Emit(OpMul)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "42" {
		t.Errorf("6 * 7 = %q, want '42'", result)
	}
}

func TestVMDiv(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("20")
	idxB := chunk.AddConstant("4")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
	chunk.Emit(OpDiv)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "5" {
		t.Errorf("20 / 4 = %q, want '5'", result)
	}
}

func TestVMDivByZero(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("42")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.Emit(OpConstZero)
	chunk.Emit(OpDiv)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "0" {
		t.Errorf("42 / 0 = %q, want '0'", result)
	}
}

func TestVMMod(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("17")
	idxB := chunk.AddConstant("5")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
	chunk.Emit(OpMod)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "2" {
		t.Errorf("17 %% 5 = %q, want '2'", result)
	}
}

func TestVMNeg(t *testing.T) {
	chunk := NewChunk()
	idx := chunk.AddConstant("42")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpNeg)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "-42" {
		t.Errorf("-42 = %q, want '-42'", result)
	}
}

// ============ Comparison Tests ============

func TestVMEq(t *testing.T) {
	tests := []struct {
		a, b   string
		expect string
	}{
		{"hello", "hello", "true"},
		{"hello", "world", "false"},
		{"42", "42", "true"},
		{"", "", "true"},
	}

	for _, tt := range tests {
		chunk := NewChunk()
		idxA := chunk.AddConstant(tt.a)
		idxB := chunk.AddConstant(tt.b)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
		chunk.Emit(OpEq)
		chunk.Emit(OpReturn)

		vm := NewVM()
		result, err := vm.Execute(chunk, "", nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if result != tt.expect {
			t.Errorf("%q == %q = %q, want %q", tt.a, tt.b, result, tt.expect)
		}
	}
}

func TestVMNe(t *testing.T) {
	chunk := NewChunk()
	idxA := chunk.AddConstant("hello")
	idxB := chunk.AddConstant("world")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
	chunk.Emit(OpNe)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("'hello' != 'world' = %q, want 'true'", result)
	}
}

func TestVMLtLeGtGe(t *testing.T) {
	tests := []struct {
		op     Opcode
		a, b   string
		expect string
	}{
		{OpLt, "3", "5", "true"},
		{OpLt, "5", "3", "false"},
		{OpLt, "5", "5", "false"},
		{OpLe, "3", "5", "true"},
		{OpLe, "5", "5", "true"},
		{OpLe, "5", "3", "false"},
		{OpGt, "5", "3", "true"},
		{OpGt, "3", "5", "false"},
		{OpGt, "5", "5", "false"},
		{OpGe, "5", "3", "true"},
		{OpGe, "5", "5", "true"},
		{OpGe, "3", "5", "false"},
	}

	for _, tt := range tests {
		chunk := NewChunk()
		idxA := chunk.AddConstant(tt.a)
		idxB := chunk.AddConstant(tt.b)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxA)...)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idxB)...)
		chunk.Emit(tt.op)
		chunk.Emit(OpReturn)

		vm := NewVM()
		result, err := vm.Execute(chunk, "", nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if result != tt.expect {
			t.Errorf("%s %s %s = %q, want %q", tt.a, tt.op.String(), tt.b, result, tt.expect)
		}
	}
}

// ============ Logical Tests ============

func TestVMNot(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"true", "false"},
		{"false", "true"},
		{"", "true"},
		{"0", "true"},
		{"nil", "true"},
		{"hello", "false"},
		{"1", "false"},
	}

	for _, tt := range tests {
		chunk := NewChunk()
		idx := chunk.AddConstant(tt.input)
		chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
		chunk.Emit(OpNot)
		chunk.Emit(OpReturn)

		vm := NewVM()
		result, err := vm.Execute(chunk, "", nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if result != tt.expect {
			t.Errorf("NOT %q = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

func TestVMAnd(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpAnd)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("true AND true = %q, want 'true'", result)
	}
}

func TestVMOr(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstFalse)
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpOr)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("false OR true = %q, want 'true'", result)
	}
}

// ============ String Tests ============

func TestVMConcat(t *testing.T) {
	chunk := NewChunk()
	idx1 := chunk.AddConstant("hello")
	idx2 := chunk.AddConstant(" world")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx1)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx2)...)
	chunk.Emit(OpConcat)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "hello world" {
		t.Errorf("concat = %q, want 'hello world'", result)
	}
}

func TestVMStrLen(t *testing.T) {
	chunk := NewChunk()
	idx := chunk.AddConstant("hello")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpStrLen)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "5" {
		t.Errorf("strlen('hello') = %q, want '5'", result)
	}
}

// ============ Control Flow Tests ============

func TestVMJump(t *testing.T) {
	chunk := NewChunk()
	// Jump over the "false" constant to "true"
	jumpPos := chunk.EmitJump(OpJump)
	chunk.Emit(OpConstFalse) // Skipped
	chunk.Emit(OpReturn)     // Skipped
	chunk.PatchJump(jumpPos)
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
}

func TestVMJumpTrue(t *testing.T) {
	chunk := NewChunk()
	// Push true, then jump
	chunk.Emit(OpConstTrue)
	jumpPos := chunk.EmitJump(OpJumpTrue)
	chunk.Emit(OpConstFalse)
	chunk.Emit(OpReturn)
	chunk.PatchJump(jumpPos)
	idx := chunk.AddConstant("jumped")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "jumped" {
		t.Errorf("Expected 'jumped', got %q", result)
	}
}

func TestVMJumpFalse(t *testing.T) {
	chunk := NewChunk()
	// Push false, then jump
	chunk.Emit(OpConstFalse)
	jumpPos := chunk.EmitJump(OpJumpFalse)
	chunk.Emit(OpConstTrue)
	chunk.Emit(OpReturn)
	chunk.PatchJump(jumpPos)
	idx := chunk.AddConstant("jumped")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "jumped" {
		t.Errorf("Expected 'jumped', got %q", result)
	}
}

func TestVMJumpNil(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstNil)
	jumpPos := chunk.EmitJump(OpJumpNil)
	chunk.Emit(OpConstFalse)
	chunk.Emit(OpReturn)
	chunk.PatchJump(jumpPos)
	idx := chunk.AddConstant("was_nil")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "was_nil" {
		t.Errorf("Expected 'was_nil', got %q", result)
	}
}

func TestVMJumpNotNil(t *testing.T) {
	chunk := NewChunk()
	idx := chunk.AddConstant("not_nil")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	jumpPos := chunk.EmitJump(OpJumpNotNil)
	chunk.Emit(OpConstFalse)
	chunk.Emit(OpReturn)
	chunk.PatchJump(jumpPos)
	idx2 := chunk.AddConstant("jumped")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx2)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "jumped" {
		t.Errorf("Expected 'jumped', got %q", result)
	}
}

// ============ Return Tests ============

func TestVMReturn(t *testing.T) {
	chunk := NewChunk()
	idx := chunk.AddConstant("result")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpReturn)
	// This should never be reached
	chunk.Emit(OpConstFalse)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "result" {
		t.Errorf("Expected 'result', got %q", result)
	}
}

func TestVMReturnNil(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstTrue) // Push something
	chunk.Emit(OpReturnNil) // Return nil instead

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty, got %q", result)
	}
}

// ============ Message Send Tests ============

func TestVMSend(t *testing.T) {
	chunk := NewChunk()
	recvIdx := chunk.AddConstant("counter_123")
	selIdx := chunk.AddConstant("getValue")

	chunk.EmitWithOperand(OpConst, uint16Bytes(recvIdx)...)
	chunk.EmitWithOperand(OpSend, uint16Bytes(selIdx)[0], uint16Bytes(selIdx)[1], 0) // argc=0
	chunk.Emit(OpReturn)

	vm := NewVM()
	vm.SetMessageSender(&MockMessageSender{
		Calls: []MockCall{
			{Receiver: "counter_123", Selector: "getValue", Result: "42"},
		},
	})

	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "42" {
		t.Errorf("Expected '42', got %q", result)
	}
}

func TestVMSendWithArgs(t *testing.T) {
	chunk := NewChunk()
	recvIdx := chunk.AddConstant("counter_123")
	selIdx := chunk.AddConstant("add:")
	argIdx := chunk.AddConstant("10")

	chunk.EmitWithOperand(OpConst, uint16Bytes(recvIdx)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(argIdx)...)
	chunk.EmitWithOperand(OpSend, uint16Bytes(selIdx)[0], uint16Bytes(selIdx)[1], 1) // argc=1
	chunk.Emit(OpReturn)

	vm := NewVM()
	vm.SetMessageSender(&MockMessageSender{
		Calls: []MockCall{
			{Receiver: "counter_123", Selector: "add:", Result: "52"},
		},
	})

	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "52" {
		t.Errorf("Expected '52', got %q", result)
	}
}

func TestVMSendSelf(t *testing.T) {
	chunk := NewChunk()
	selIdx := chunk.AddConstant("value")

	chunk.Emit(OpConstNil) // Push placeholder for self
	chunk.EmitWithOperand(OpSendSelf, uint16Bytes(selIdx)[0], uint16Bytes(selIdx)[1], 0) // argc=0
	chunk.Emit(OpReturn)

	vm := NewVM()
	vm.SetMessageSender(&MockMessageSender{
		Calls: []MockCall{
			{Receiver: "counter_abc", Selector: "value", Result: "99"},
		},
	})

	result, err := vm.Execute(chunk, "counter_abc", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "99" {
		t.Errorf("Expected '99', got %q", result)
	}
}

func TestVMSendNoSender(t *testing.T) {
	chunk := NewChunk()
	recvIdx := chunk.AddConstant("obj")
	selIdx := chunk.AddConstant("method")

	chunk.EmitWithOperand(OpConst, uint16Bytes(recvIdx)...)
	chunk.EmitWithOperand(OpSend, uint16Bytes(selIdx)[0], uint16Bytes(selIdx)[1], 0)
	chunk.Emit(OpReturn)

	vm := NewVM()
	// No sender configured

	_, err := vm.Execute(chunk, "", nil)
	if err == nil {
		t.Error("Expected error for missing message sender")
	}
}

// ============ Array Operation Tests ============

func TestVMArrayNew(t *testing.T) {
	chunk := chunkWithCode(byte(OpArrayNew), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "[]" {
		t.Errorf("Expected '[]', got %q", result)
	}
}

func TestVMArrayPush(t *testing.T) {
	chunk := NewChunk()
	idx := chunk.AddConstant("hello")

	chunk.Emit(OpArrayNew)
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	chunk.Emit(OpArrayPush)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != `["hello"]` {
		t.Errorf("Expected '[\"hello\"]', got %q", result)
	}
}

func TestVMArrayAt(t *testing.T) {
	chunk := NewChunk()
	arrIdx := chunk.AddConstant(`["a","b","c"]`)

	chunk.EmitWithOperand(OpConst, uint16Bytes(arrIdx)...)
	chunk.Emit(OpConstOne)
	chunk.Emit(OpArrayAt)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "b" {
		t.Errorf("Expected 'b', got %q", result)
	}
}

func TestVMArrayLen(t *testing.T) {
	chunk := NewChunk()
	arrIdx := chunk.AddConstant(`["a","b","c"]`)

	chunk.EmitWithOperand(OpConst, uint16Bytes(arrIdx)...)
	chunk.Emit(OpArrayLen)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "3" {
		t.Errorf("Expected '3', got %q", result)
	}
}

func TestVMArrayFirst(t *testing.T) {
	chunk := NewChunk()
	arrIdx := chunk.AddConstant(`["first","second"]`)

	chunk.EmitWithOperand(OpConst, uint16Bytes(arrIdx)...)
	chunk.Emit(OpArrayFirst)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "first" {
		t.Errorf("Expected 'first', got %q", result)
	}
}

func TestVMArrayLast(t *testing.T) {
	chunk := NewChunk()
	arrIdx := chunk.AddConstant(`["first","last"]`)

	chunk.EmitWithOperand(OpConst, uint16Bytes(arrIdx)...)
	chunk.Emit(OpArrayLast)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "last" {
		t.Errorf("Expected 'last', got %q", result)
	}
}

// ============ Object Operation Tests ============

func TestVMObjectNew(t *testing.T) {
	chunk := chunkWithCode(byte(OpObjectNew), byte(OpReturn))

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "{}" {
		t.Errorf("Expected '{}', got %q", result)
	}
}

func TestVMObjectAtPut(t *testing.T) {
	chunk := NewChunk()
	keyIdx := chunk.AddConstant("name")
	valIdx := chunk.AddConstant("test")

	chunk.Emit(OpObjectNew)
	chunk.EmitWithOperand(OpConst, uint16Bytes(keyIdx)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(valIdx)...)
	chunk.Emit(OpObjectAtPut)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != `{"name":"test"}` {
		t.Errorf("Expected '{\"name\":\"test\"}', got %q", result)
	}
}

func TestVMObjectAt(t *testing.T) {
	chunk := NewChunk()
	objIdx := chunk.AddConstant(`{"foo":"bar"}`)
	keyIdx := chunk.AddConstant("foo")

	chunk.EmitWithOperand(OpConst, uint16Bytes(objIdx)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(keyIdx)...)
	chunk.Emit(OpObjectAt)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "bar" {
		t.Errorf("Expected 'bar', got %q", result)
	}
}

func TestVMObjectHasKey(t *testing.T) {
	chunk := NewChunk()
	objIdx := chunk.AddConstant(`{"foo":"bar"}`)
	keyIdx := chunk.AddConstant("foo")

	chunk.EmitWithOperand(OpConst, uint16Bytes(objIdx)...)
	chunk.EmitWithOperand(OpConst, uint16Bytes(keyIdx)...)
	chunk.Emit(OpObjectHasKey)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
}

// ============ Unknown Opcode Test ============

func TestVMUnknownOpcode(t *testing.T) {
	chunk := chunkWithCode(0xEE) // Undefined opcode

	vm := NewVM()
	_, err := vm.Execute(chunk, "", nil)
	if err == nil {
		t.Error("Expected error for unknown opcode")
	}
}

// ============ Integration Tests ============

func TestVMSimpleLoop(t *testing.T) {
	// Implement: sum = 0; i = 0; while (i < 5) { sum = sum + i; i = i + 1 }; return sum
	chunk := NewChunk()
	chunk.LocalCount = 2 // slot 0 = sum, slot 1 = i

	// sum := 0
	chunk.Emit(OpConstZero)
	chunk.EmitWithOperand(OpStoreLocal, 0)

	// i := 0
	chunk.Emit(OpConstZero)
	chunk.EmitWithOperand(OpStoreLocal, 1)

	// Loop start (offset 6)
	loopStart := len(chunk.Code)

	// i < 5
	chunk.EmitWithOperand(OpLoadLocal, 1)
	idx5 := chunk.AddConstant("5")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx5)...)
	chunk.Emit(OpLt)

	// Jump to end if false
	exitJump := chunk.EmitJump(OpJumpFalse)

	// sum := sum + i
	chunk.EmitWithOperand(OpLoadLocal, 0)
	chunk.EmitWithOperand(OpLoadLocal, 1)
	chunk.Emit(OpAdd)
	chunk.EmitWithOperand(OpStoreLocal, 0)

	// i := i + 1
	chunk.EmitWithOperand(OpLoadLocal, 1)
	chunk.Emit(OpConstOne)
	chunk.Emit(OpAdd)
	chunk.EmitWithOperand(OpStoreLocal, 1)

	// Loop back
	chunk.EmitLoop(loopStart)

	// Exit
	chunk.PatchJump(exitJump)

	// Return sum
	chunk.EmitWithOperand(OpLoadLocal, 0)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 0 + 1 + 2 + 3 + 4 = 10
	if result != "10" {
		t.Errorf("Expected '10', got %q", result)
	}
}

func TestVMConditional(t *testing.T) {
	// if (true) { return "yes" } else { return "no" }
	chunk := NewChunk()

	chunk.Emit(OpConstTrue)
	falseJump := chunk.EmitJump(OpJumpFalse)

	// True branch
	yesIdx := chunk.AddConstant("yes")
	chunk.EmitWithOperand(OpConst, uint16Bytes(yesIdx)...)
	chunk.Emit(OpReturn)

	chunk.PatchJump(falseJump)

	// False branch
	noIdx := chunk.AddConstant("no")
	chunk.EmitWithOperand(OpConst, uint16Bytes(noIdx)...)
	chunk.Emit(OpReturn)

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "yes" {
		t.Errorf("Expected 'yes', got %q", result)
	}
}

func TestVMImplicitReturn(t *testing.T) {
	// No explicit return - should return stack top
	chunk := NewChunk()
	idx := chunk.AddConstant("implicit")
	chunk.EmitWithOperand(OpConst, uint16Bytes(idx)...)
	// No OpReturn

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "implicit" {
		t.Errorf("Expected 'implicit', got %q", result)
	}
}

func TestVMImplicitReturnEmpty(t *testing.T) {
	// Empty chunk - should return empty
	chunk := NewChunk()

	vm := NewVM()
	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty, got %q", result)
	}
}

// ============ CaptureCell Tests ============

func TestCaptureCellBasic(t *testing.T) {
	cell := &CaptureCell{
		Value:  "initial",
		Name:   "x",
		Source: VarSourceLocal,
	}

	if cell.Get() != "initial" {
		t.Errorf("Expected 'initial', got %q", cell.Get())
	}

	cell.Set("updated")
	if cell.Get() != "updated" {
		t.Errorf("Expected 'updated', got %q", cell.Get())
	}
}

func TestCaptureCellNil(t *testing.T) {
	var cell *CaptureCell = nil

	// Should not panic
	if cell.Get() != "" {
		t.Errorf("Expected empty for nil cell")
	}
	cell.Set("value") // Should not panic
	cell.Close()      // Should not panic
	cell.Retain()     // Should not panic
	cell.Release()    // Should not panic
}

func TestCaptureCellIVarWriteback(t *testing.T) {
	accessor := NewMockInstanceAccessor()
	accessor.Vars["obj_123"] = map[string]string{"count": "10"}

	cell := &CaptureCell{
		Value:      "",
		Name:       "count",
		Source:     VarSourceIVar,
		InstanceID: "obj_123",
		Accessor:   accessor,
	}

	// Get should read from accessor
	if cell.Get() != "10" {
		t.Errorf("Expected '10', got %q", cell.Get())
	}

	// Set should write to accessor
	cell.Set("20")
	if accessor.Vars["obj_123"]["count"] != "20" {
		t.Errorf("Expected ivar to be '20', got %q", accessor.Vars["obj_123"]["count"])
	}
}

func TestCaptureCellClose(t *testing.T) {
	accessor := NewMockInstanceAccessor()
	accessor.Vars["obj_123"] = map[string]string{"count": "10"}

	cell := &CaptureCell{
		Value:      "",
		Name:       "count",
		Source:     VarSourceIVar,
		InstanceID: "obj_123",
		Accessor:   accessor,
	}

	cell.Close()

	// After close, should have snapshotted value
	if cell.Value != "10" {
		t.Errorf("Expected snapshotted value '10', got %q", cell.Value)
	}

	// After close, set should not write back
	accessor.Vars["obj_123"]["count"] = "50"
	if cell.Get() != "10" {
		t.Errorf("Expected cached '10', got %q", cell.Get())
	}
}

func TestCaptureCellRefCount(t *testing.T) {
	cell := &CaptureCell{
		Value:    "test",
		RefCount: 0,
	}

	cell.Retain()
	if cell.RefCount != 1 {
		t.Errorf("Expected refcount 1, got %d", cell.RefCount)
	}

	cell.Retain()
	if cell.RefCount != 2 {
		t.Errorf("Expected refcount 2, got %d", cell.RefCount)
	}

	cell.Release()
	if cell.RefCount != 1 {
		t.Errorf("Expected refcount 1, got %d", cell.RefCount)
	}
}

// ============ Trace Mode Test ============

func TestVMTraceMode(t *testing.T) {
	chunk := NewChunk()
	chunk.Emit(OpConstOne)
	chunk.Emit(OpConstOne)
	chunk.Emit(OpAdd)
	chunk.Emit(OpReturn)

	vm := NewVM()
	vm.Trace = true // Enable trace (output goes to stdout, but shouldn't panic)

	result, err := vm.Execute(chunk, "", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "2" {
		t.Errorf("Expected '2', got %q", result)
	}
}
