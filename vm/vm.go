package vm

import (
	"math/rand"
)

// --- Constants ---
const (
	NumRegisters      = 4
	GenomeSnippetSize = 32
)

// --- Instruction Set Opcodes ---
const (
	NOOP byte = iota
	SET
	COPY
	ADD
	SUB
	JUMP_REL_IF_ZERO
	READ_REL
	WRITE_REL
	NumOpcodes // This must be the last entry, it counts the number of opcodes
)

var OpcodeNames = [...]string{
	"NOOP",
	"SET",
	"COPY",
	"ADD",
	"SUB",
	"JUMP_REL_IF_ZERO",
	"READ_REL",
	"WRITE_REL",
}

// IP represents an Instruction Pointer, our digital organism.
// It holds its own registers and all the tools needed to spawn a child.
type IP struct {
	ID         int
	Registers  [NumRegisters]int32
	CurrentPtr int32 // Current instruction pointer in the soup
	Steps      int64 // Number of steps executed
	Soup       []int32
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID           int
	Registers    [NumRegisters]int32
	CurrentPtr   int32
	Steps        int64
}

// Savable returns a serializable representation of the IP.
func (ip *IP) Savable() SavableIP {
	return SavableIP{
		ID:           ip.ID,
		CurrentPtr:   ip.CurrentPtr,
		Registers:    ip.Registers,
		Steps:        ip.Steps,
	}
}

// NewIP creates a new, minimal instruction pointer.
// The concurrency tools (Population, Wg, etc.) must be set by the main loop.
func NewIP(id int, soup []int32, startPtr int32) *IP {
	return &IP{
		ID:               id,
		Soup:             soup,
		CurrentPtr:       startPtr,
	}
}

// Step executes a single instruction from the soup.
func (ip *IP) Step() {

	soupLen := int32(len(ip.Soup))

	// Jump to a random part of the soup
	// NOTE make this based on a timer rather than steps to
	// tie it to the parent universe (the physical machine)
	if (ip.Steps % 10000 == 0 ){
		ip.CurrentPtr = int32(rand.Intn(len(ip.Soup)))
	}

	// The pointer should always be valid at the start of a step due to the wrap at the end.
	// Helper function to safely read a value from the soup, handling pointer wrap.
	safeReadSoup := func() int32 {
		val := ip.Soup[(ip.CurrentPtr%soupLen+soupLen)%soupLen]
		ip.CurrentPtr++
		return val
	}

	opcode := (safeReadSoup()%int32(NumOpcodes) + int32(NumOpcodes)) % int32(NumOpcodes)
	originalPtr := ip.CurrentPtr

	// Helper function to safely read a register index from the soup.
	safeRegIndex := func() int32 {
		val := safeReadSoup()
		return (val%NumRegisters + NumRegisters) % NumRegisters
	}

	// --- Instruction Decoder ---
	switch byte(opcode) {
	case NOOP:
	// PC already incremented
	case SET:
		regDest := safeRegIndex()
		value := safeReadSoup()
		ip.Registers[regDest] = value
	case COPY:
		regDest := safeRegIndex()
		regSrc := safeRegIndex()
		ip.Registers[regDest] = ip.Registers[regSrc]
	case ADD:
		regDest := safeRegIndex()
		regSrc1 := safeRegIndex()
		regSrc2 := safeRegIndex()
		ip.Registers[regDest] = ip.Registers[regSrc1] + ip.Registers[regSrc2]
	case SUB:
		regDest := safeRegIndex()
		regSrc1 := safeRegIndex()
		regSrc2 := safeRegIndex()
		ip.Registers[regDest] = ip.Registers[regSrc1] - ip.Registers[regSrc2]
	case JUMP_REL_IF_ZERO:
		regCond := safeRegIndex()
		regOffset := safeRegIndex() // Always read both arguments
		if ip.Registers[regCond] == 0 {
			// Jump relative to the current pointer (which is already past the arguments)
			ip.CurrentPtr += ip.Registers[regOffset]
		}
	case READ_REL:
		regDest := safeRegIndex()
		regOffset := safeRegIndex()
		readAddr := originalPtr + ip.Registers[regOffset]
		readAddr = (readAddr%soupLen + soupLen) % soupLen // Wrap address
		ip.Registers[regDest] = ip.Soup[readAddr]
	case WRITE_REL:
		regSrc := safeRegIndex()
		regOffset := safeRegIndex()
		writeAddr := originalPtr + ip.Registers[regOffset]
		writeAddr = (writeAddr%soupLen + soupLen) % soupLen // Wrap address
		ip.Soup[writeAddr] = ip.Registers[regSrc]
	}

	// Wrap the final pointer to ensure it's always valid.
	ip.CurrentPtr = (ip.CurrentPtr%soupLen + soupLen) % soupLen
	ip.Steps++
}
