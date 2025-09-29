package vm

import (
	"encoding/binary"
)

// --- Instruction Set Opcodes ---
const (
	NOOP int8 = iota // 0
	PUSH_I           // 1
	POP_A            // 2
	PUSH_A           // 3
	INC              // 4
	DEC              // 5
	XOR              // 6
	SHF              // 7
	INV              // 8
	ADD              // 9
	SUB              // 10
	AND              // 11
	OR               // 12
	JMP_Z            // 13
	JMP_NZ           // 14
	SET_SP           // 15
	NumOpcodes
)

var OpcodeNames = [...]string{
	"NOOP",
	"PUSH_I",
	"POP_A",
	"PUSH_A",
	"INC",
	"DEC",
	"XOR",
	"SHF",
	"INV",
	"ADD",
	"SUB",
	"AND",
	"OR",
	"JMP_Z",
	"JMP_NZ",
	"SET_SP",
}

// IP represents an Instruction Pointer, our digital organism.
type IP struct {
	ID                      int
	CurrentPtr              int32 // Current instruction pointer in the soup
	StackPointer            int32 // Stack pointer in the soup
	Steps                   int64 // Number of steps executed
	Soup                    []int8
	Use32BitAddressing      bool
	UseRelativeAddressing   bool
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID                 int
	CurrentPtr         int32
	StackPointer       int32
	Steps              int64
	CurrentInstruction int8 // The raw instruction byte at CurrentPtr
}

// CurrentState returns a serializable representation of the IP.
func (ip *IP) CurrentState() SavableIP {
	// Ensure CurrentPtr is within bounds for accessing Soup
	soupLen := int32(len(ip.Soup))
	wrappedCurrentPtr := (ip.CurrentPtr%soupLen + soupLen) % soupLen

	return SavableIP{
		ID:                 ip.ID,
		CurrentPtr:         ip.CurrentPtr,
		StackPointer:       ip.StackPointer,
		Steps:              ip.Steps,
		CurrentInstruction: ip.Soup[wrappedCurrentPtr],
	}
}

// NewIP creates a new, minimal instruction pointer.
func NewIP(id int, soup []int8, startPtr int32, use32BitAddressing bool, useRelativeAddressing bool) *IP {
	ip := &IP{
		ID:                      id,
		Soup:                    soup,
		CurrentPtr:              startPtr,
		StackPointer:            int32(len(soup)),
		Use32BitAddressing:      use32BitAddressing,
		UseRelativeAddressing:   useRelativeAddressing,
	}
	return ip
}

// Step executes a single instruction from the soup.
func (ip *IP) Step() {
	soupLen := int32(len(ip.Soup))

	// --- Helper Functions ---
	wrapAddr := func(addr int32) int32 {
		return (addr%soupLen + soupLen) % soupLen
	}

	// Pushes a value onto the stack.
	push := func(val int8) {
		ip.StackPointer--
		ip.Soup[wrapAddr(ip.StackPointer)] = val
	}

	// Pops a value from the stack.
	pop := func() int8 {
		val := ip.Soup[wrapAddr(ip.StackPointer)]
		ip.StackPointer++
		return val
	}

	// Pops 4 bytes from the stack and converts them to a 32-bit integer.
	pop32 := func() int32 {
		b4 := byte(pop())
		b3 := byte(pop())
		b2 := byte(pop())
		b1 := byte(pop())
		byteSlice := []byte{b1, b2, b3, b4}
		return int32(binary.BigEndian.Uint32(byteSlice))
	}

	// Fetches 1 byte from the instruction stream and advances the pointer.
	fetch8 := func() int8 {
		val := ip.Soup[wrapAddr(ip.CurrentPtr)]
		ip.CurrentPtr = wrapAddr(ip.CurrentPtr + 1)
		return val
	}

	// Fetches 4 bytes and converts them to a 32-bit integer.
	fetch32 := func() int32 {
		b1 := byte(fetch8())
		b2 := byte(fetch8())
		b3 := byte(fetch8())
		b4 := byte(fetch8())
		byteSlice := []byte{b1, b2, b3, b4}
		return int32(binary.BigEndian.Uint32(byteSlice))
	}

	// Fetches an immediate operand from the instruction stream.
	fetchImmediate := func() int32 {
		if ip.Use32BitAddressing {
			return fetch32()
		} else {
			return int32(fetch8())
		}
	}

	// Calculates the final address for an operation based on the addressing mode.
	resolveAddress := func(baseAddr int32, offset int32) int32 {
		if ip.UseRelativeAddressing {
			return wrapAddr(baseAddr + offset)
		} else {
			return wrapAddr(offset)
		}
	}


	opcodeLocation := ip.CurrentPtr

	// --- Instruction Execution ---
	opcode := fetch8()

	switch opcode {
	case NOOP:
		// Does nothing.
	case PUSH_I:
		val := fetch8()
		push(val)
	case POP_A:
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] = pop()
	case PUSH_A:
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		push(ip.Soup[addr])
	case INC:
		val := pop()
		push(val + 1)
	case DEC:
		val := pop()
		push(val - 1)
	case XOR:
		val2 := pop()
		val1 := pop()
		push(val1 ^ val2)
	case SHF:
		shiftByte := fetch8()
		val := pop()
		direction := (shiftByte >> 7) & 1 // Use the MSB for direction
		amount := uint(shiftByte & 0x7F)    // Use the other 7 bits for amount

		if direction == 0 { // Left shift
			val <<= amount
		} else { // Right shift
			val >>= amount
		}
		push(val)
	case INV:
		val := pop()
		push(^val)
	case ADD:
		val2 := pop()
		val1 := pop()
		push(val1 + val2)
	case SUB:
		val2 := pop()
		val1 := pop()
		push(val1 - val2)
	case AND:
		val2 := pop()
		val1 := pop()
		push(val1 & val2)
	case OR:
		val2 := pop()
		val1 := pop()
		push(val1 | val2)
	case JMP_Z:
		jumpOffset := fetchImmediate()
		if pop() == 0 {
			ip.CurrentPtr = resolveAddress(opcodeLocation, jumpOffset)
		}
	case JMP_NZ:
		jumpOffset := fetchImmediate()
		if pop() != 0 {
			ip.CurrentPtr = resolveAddress(opcodeLocation, jumpOffset)
		}
	case SET_SP:
		var newSPValue int32
		if ip.Use32BitAddressing {
			newSPValue = pop32()
		} else {
			newSPValue = int32(pop())
		}
		ip.StackPointer = resolveAddress(opcodeLocation, newSPValue)
	}
	ip.Steps++
}
