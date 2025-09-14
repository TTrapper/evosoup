package vm

import (
	"encoding/binary"
	"math/rand"
)

// --- Instruction Set Opcodes ---
const (
	NOOP int8 = iota // 0
	MOV              // 1
	WRT              // 2
	INC              // 3
	DEC              // 4
	XOR              // 5
	SHF              // 6
	INV              // 7
	ADD              // 8
	SUB              // 9
	AND              // 10
	OR               // 11
	JMP_Z            // 12
	JMP_NZ           // 13
	NumOpcodes
)

var OpcodeNames = [...]string{
	"NOOP",
	"MOV",
	"WRT",
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
	case MOV:
		srcOffset := fetchImmediate()
		destOffset := fetchImmediate()
		srcAddr := resolveAddress(opcodeLocation, srcOffset)
		destAddr := resolveAddress(opcodeLocation, destOffset)
		ip.Soup[destAddr] = ip.Soup[srcAddr]
	case WRT:
		destOffset := fetchImmediate()
		value := fetch8()
		destAddr := resolveAddress(opcodeLocation, destOffset)
		ip.Soup[destAddr] = value
	case INC:
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr]++
	case DEC:
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr]--
	case XOR:
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] ^= value
	case SHF:
		// NOTE this op causes a 'grid of activity' when in 32-bit absolute mode
		offset := fetchImmediate()
		shiftByte := fetch8()
		direction := (shiftByte >> 7) & 1 // Use the MSB for direction
		amount := uint(shiftByte & 0x7F)    // Use the other 7 bits for amount

		addr := resolveAddress(opcodeLocation, offset)

		if direction == 0 { // Left shift
			ip.Soup[addr] <<= amount
		} else { // Right shift
			ip.Soup[addr] >>= amount
		}
	case INV:
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] = ^ip.Soup[addr]
	case ADD:
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] += value
	case SUB:
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] -= value
	case AND:
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] &= value
	case OR:
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] |= value
	case JMP_Z:
		addrOffset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, addrOffset)

		if ip.Soup[addr] == 0 {
			var randomJumpOffset int32
			if ip.Use32BitAddressing {
				randomJumpOffset = int32(rand.Uint32())
			} else {
				randomJumpOffset = int32(int8(rand.Intn(256)))
			}
			ip.CurrentPtr = resolveAddress(opcodeLocation, randomJumpOffset)
		}
	case JMP_NZ:
		addrOffset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, addrOffset)

		if ip.Soup[addr] != 0 {
			var randomJumpOffset int32
			if ip.Use32BitAddressing {
				randomJumpOffset = int32(rand.Uint32())
			} else {
				randomJumpOffset = int32(int8(rand.Intn(256)))
			}
			ip.CurrentPtr = resolveAddress(opcodeLocation, randomJumpOffset)
		}
	}
	ip.Steps++
}
