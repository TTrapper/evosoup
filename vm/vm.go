package vm

import (
	"encoding/binary"
	"math/rand"
)

// --- Instruction Set Opcodes ---
const (
	NOOP int8 = iota // 0
	POP_I            // 1
	PUSH_I           // 2
	POP_A            // 3
	PUSH_A           // 4
	INC              // 5
	DEC              // 6
	XOR              // 7
	SHF              // 8
	INV              // 9
	ADD              // 10
	SUB              // 11
	AND              // 12
	OR               // 13
	JMP_Z            // 14
	JMP_NZ           // 15
	SET_SP           // 16
	NumOpcodes
)

var OpcodeNames = [...]string{
	"NOOP", "POP_I", "PUSH_I", "POP_A", "PUSH_A", "INC", "DEC", "XOR", "SHF", "INV", "ADD", "SUB", "AND", "OR", "JMP_Z", "JMP_NZ", "SET_SP",
}

var directions = [8][2]int32{
	{-1, -1}, {-1, 0}, {-1, 1},
	{0, -1}, {0, 1},
	{1, -1}, {1, 0}, {1, 1},
}

// IP represents an Instruction Pointer, our digital organism.
type IP struct {
	ID                    int
	X, Y                  int32 // Current instruction pointer in the soup
	StackPointer          int32 // Stack pointer in the soup
	Steps                 int64 // Number of steps executed
	Soup                  []int8
	Use32BitAddressing    bool
	UseRelativeAddressing bool
	SoupDimX              int32
	SoupDimY              int32
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID                 int
	X, Y               int32
	StackPointer       int32
	Steps              int64
	CurrentInstruction int8 // The raw instruction byte at CurrentPtr
}

func (ip *IP) wrap(val, max int32) int32 {
	return (val%max + max) % max
}

func (ip *IP) to1D(x, y int32) int32 {
	return ip.wrap(y, ip.SoupDimY)*ip.SoupDimX + ip.wrap(x, ip.SoupDimX)
}

// CurrentState returns a serializable representation of the IP.
func (ip *IP) CurrentState() SavableIP {
	addr := ip.to1D(ip.X, ip.Y)
	return SavableIP{
		ID:                 ip.ID,
		X:                  ip.X,
		Y:                  ip.Y,
		StackPointer:       ip.StackPointer,
		Steps:              ip.Steps,
		CurrentInstruction: ip.Soup[addr],
	}
}

// NewIP creates a new, minimal instruction pointer.
func NewIP(id int, soup []int8, x, y, soupDimX int32, use32BitAddressing bool, useRelativeAddressing bool) *IP {
	ip := &IP{
		ID:                    id,
		Soup:                  soup,
		X:                     x,
		Y:                     y,
		StackPointer:          int32(len(soup)),
		Use32BitAddressing:    use32BitAddressing,
		UseRelativeAddressing: useRelativeAddressing,
		SoupDimX:              soupDimX,
		SoupDimY:              int32(len(soup)) / soupDimX,
	}
	return ip
}

// Step executes a single instruction from the soup.
func (ip *IP) Step() {
	soupLen := int32(len(ip.Soup))
	// --- Helper Functions ---
	// Pushes a value onto the stack.
	push := func(val int8) {
		ip.StackPointer--
		ip.Soup[(ip.StackPointer%soupLen+soupLen)%soupLen] = val
	}

	// Pops a value from the stack.
	pop := func() int8 {
		val := ip.Soup[(ip.StackPointer%soupLen+soupLen)%soupLen]
		ip.StackPointer++
		return val
	}

	// Instruction pointer for operand fetching for this step
	fetchX, fetchY := ip.X, ip.Y

	// Helper to advance fetch pointer
	advanceFetch := func() {
		fetchX++
		if fetchX >= ip.SoupDimX {
			fetchX = 0
			fetchY++
		}
		fetchY = ip.wrap(fetchY, ip.SoupDimY)
	}

	// Fetch opcode and advance fetch pointer
	opcode := ip.Soup[ip.to1D(fetchX, fetchY)]
	advanceFetch()

	// Fetches 1 byte from the instruction stream and advances the pointer.
	fetch8 := func() int8 {
		val := ip.Soup[ip.to1D(fetchX, fetchY)]
		advanceFetch()
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
	resolveAddress := func(baseX, baseY int32, offset int32) (int32, int32) {
		var finalX, finalY int32
		if ip.UseRelativeAddressing {
			if ip.Use32BitAddressing {
				dx := int32(int16(offset & 0xFFFF))
				dy := int32(int16(offset >> 16))
				finalX = baseX + dx
				finalY = baseY + dy
			} else { // 8-bit relative
				dx := int32(int8(byte(offset) << 4) >> 4) // sign extend low nibble
				dy := int32(int8(byte(offset)) >> 4)      // sign extend high nibble
				finalX = baseX + dx
				finalY = baseY + dy
			}
		} else { // Absolute addressing
			if ip.Use32BitAddressing {
				finalX = int32(uint16(offset & 0xFFFF))
				finalY = int32(uint16(offset >> 16))
			} else { // 8-bit absolute
				finalX = offset & 0x0F
				finalY = (offset >> 4) & 0x0F
			}
		}
		return ip.wrap(finalX, ip.SoupDimX), ip.wrap(finalY, ip.SoupDimY)
	}

	jumped := false
	opcodeX, opcodeY := ip.X, ip.Y // base for relative addressing

	// --- Instruction Execution ---
	switch opcode {
	case NOOP:
		// Does nothing.
	case POP_I:
		ip.Soup[ip.to1D(fetchX, fetchY)] = pop()
	case PUSH_I:
		val := fetch8()
		push(val)
	case POP_A:
		offset := fetchImmediate()
		x, y := resolveAddress(opcodeX, opcodeY, offset)
		ip.Soup[ip.to1D(x, y)] = pop()
	case PUSH_A:
		offset := fetchImmediate()
		x, y := resolveAddress(opcodeX, opcodeY, offset)
		push(ip.Soup[ip.to1D(x, y)])
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
			ip.X, ip.Y = resolveAddress(opcodeX, opcodeY, jumpOffset)
			jumped = true
		}
	case JMP_NZ:
		jumpOffset := fetchImmediate()
		if pop() != 0 {
			ip.X, ip.Y = resolveAddress(opcodeX, opcodeY, jumpOffset)
			jumped = true
		}
	case SET_SP:
		offset := fetchImmediate()
		x, y := resolveAddress(opcodeX, opcodeY, offset)
		ip.StackPointer = ip.to1D(x, y)
	}

	if !jumped {
		// The instruction finished, we move the "real" IP randomly
		dir := rand.Intn(8)
		dx := directions[dir][0]
		dy := directions[dir][1]
		ip.X += dx
		ip.Y += dy
	}

	// Wrap IP coordinates
	ip.X = ip.wrap(ip.X, ip.SoupDimX)
	ip.Y = ip.wrap(ip.Y, ip.SoupDimY)

	ip.Steps++
}