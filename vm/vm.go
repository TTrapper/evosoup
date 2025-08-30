package vm

import (
	"math"
	"math/rand"
	"sync/atomic"
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
	JMP_Z            // 8
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
	"JMP_Z",
}

// IP represents an Instruction Pointer, our digital organism.
type IP struct {
	ID                      int
	CurrentPtr              int32 // Current instruction pointer in the soup
	Steps                   int64 // Number of steps executed
	Soup                    []int8
	Use32BitAddressing      bool
	UseRelativeAddressing   bool
	JumpZFailureProbability *uint64
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID                 int
	CurrentPtr         int32
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
		Steps:              ip.Steps,
		CurrentInstruction: ip.Soup[wrappedCurrentPtr],
	}
}

// NewIP creates a new, minimal instruction pointer.
func NewIP(id int, soup []int8, startPtr int32, use32BitAddressing bool, useRelativeAddressing bool, jumpZFailureProbability *uint64) *IP {
	ip := &IP{
		ID:                      id,
		Soup:                    soup,
		CurrentPtr:              startPtr,
		Use32BitAddressing:      use32BitAddressing,
		UseRelativeAddressing:   useRelativeAddressing,
		JumpZFailureProbability: jumpZFailureProbability,
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

	// Fetches 4 bytes from the instruction stream and advances the pointer.
	fetch32 := func() int32 {
		var val int32
		val = int32(fetch8()) << 24
		val |= int32(fetch8()) << 16
		val |= int32(fetch8()) << 8
		val |= int32(fetch8())
		return val
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
	opcode := (int32(fetch8())%int32(NumOpcodes) + int32(NumOpcodes)) % int32(NumOpcodes)

	switch byte(opcode) {
	case byte(NOOP):
		// Does nothing.
	case byte(MOV):
		srcOffset := fetchImmediate()
		destOffset := fetchImmediate()
		srcAddr := resolveAddress(opcodeLocation, srcOffset)
		destAddr := resolveAddress(opcodeLocation, destOffset)
		ip.Soup[destAddr] = ip.Soup[srcAddr]
	case byte(WRT):
		destOffset := fetchImmediate()
		value := fetch8()
		destAddr := resolveAddress(opcodeLocation, destOffset)
		ip.Soup[destAddr] = value
	case byte(INC):
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr]++
	case byte(DEC):
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr]--
	case byte(XOR):
		offset := fetchImmediate()
		value := fetch8()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] ^= value
	case byte(JMP_Z):
		addrOffset := fetchImmediate()
		jumpOffset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, addrOffset)

		prob := math.Float64frombits(atomic.LoadUint64(ip.JumpZFailureProbability))
		if rand.Float64() >= prob {
			if ip.Soup[addr] == 0 {
				ip.CurrentPtr = wrapAddr(opcodeLocation + jumpOffset)
			}
		}
	case byte(SHF):
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
	case byte(INV):
		offset := fetchImmediate()
		addr := resolveAddress(opcodeLocation, offset)
		ip.Soup[addr] = ^ip.Soup[addr]
	}
	ip.Steps++
}
