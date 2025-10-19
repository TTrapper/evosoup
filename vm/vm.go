package vm

import (
	"encoding/binary"
	"math/rand"
)

// --- Instruction Set Opcodes ---
// 5 bits for opcode, 3 bits for addressing modes.
const (
	OP_MOV uint8 = (iota + 1) << 4 // 16
	OP_ADD                       // 32
	OP_SUB                       // 48
	OP_INC                       // 64
	OP_DEC                       // 80
	OP_XOR                       // 96
	OP_AND                       // 112
	OP_OR                        // 128
	OP_SHF                       // 144
	OP_JMP                       // 160
	OP_JE                        // 176 // Jump if zero
	OP_JNE                       // 192 // Jump if not zero
)

var OpcodeNames = [...]string{
	"NOOP", "MOV", "ADD", "SUB", "INC", "DEC", "XOR", "AND", "OR", "SHF", "INV", "JMP", "JE", "JNE",
}

// OpcodeInfo contains the name and value for a given opcode.
type OpcodeInfo struct {
	Name  string `json:"name"`
	Value uint8   `json:"value"`
}

// GetOpcodes returns a list of all defined opcodes and their values.
func GetOpcodes() []OpcodeInfo {
	return []OpcodeInfo{
		{Name: "MOV", Value: OP_MOV},
		{Name: "ADD", Value: OP_ADD},
		{Name: "SUB", Value: OP_SUB},
		{Name: "INC", Value: OP_INC},
		{Name: "DEC", Value: OP_DEC},
		{Name: "XOR", Value: OP_XOR},
		{Name: "AND", Value: OP_AND},
		{Name: "OR", Value: OP_OR},
		{Name: "SHF", Value: OP_SHF},
		{Name: "JMP", Value: OP_JMP},
		{Name: "JE", Value: OP_JE},
		{Name: "JNE", Value: OP_JNE},
	}
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
		Use32BitAddressing:    use32BitAddressing,
		UseRelativeAddressing: useRelativeAddressing,
		SoupDimX:              soupDimX,
		SoupDimY:              int32(len(soup)) / soupDimX,
	}
	return ip
}

// Step executes a single instruction from the soup.
func (ip *IP) Step() {
	// --- Helper Functions ---
	// Instruction pointer for operand fetching for this step
	fetchX, fetchY := ip.X, ip.Y
	opcodeX, opcodeY := ip.X, ip.Y // base for relative addressing

	// Helper to advance fetch pointer
	advanceFetch := func() {
		fetchX++
		if fetchX >= ip.SoupDimX {
			fetchX = 0
			fetchY++
		}
		fetchY = ip.wrap(fetchY, ip.SoupDimY)
	}

	// Fetch instruction byte and advance fetch pointer
	instr := uint8(ip.Soup[ip.to1D(fetchX, fetchY)])
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
				dx := int32(int8(byte(offset) << 4) >> 4)
				dy := int32(int8(byte(offset)) >> 4)
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

	opcode := instr &^ 0x0F
	invertBit := (instr >> 3) & 1
	modeDst := (instr >> 2) & 1
	modeSrc2 := (instr >> 1) & 1
	modeSrc1 := (instr >> 0) & 1

	getOperand := func(mode uint8) int32 {
		if mode == 1 { // Immediate
			return fetchImmediate()
		} else { // Address
			offset := fetchImmediate()
			x, y := resolveAddress(opcodeX, opcodeY, offset)
			return int32(ip.Soup[ip.to1D(x, y)])
		}
	}

	getDestAddress := func(mode uint8) int32 {
		if mode == 1 { // "Immediate" destination -> self-modifying
			return ip.to1D(fetchX, fetchY)
		} else { // Address destination
			offset := fetchImmediate()
			x, y := resolveAddress(opcodeX, opcodeY, offset)
			return ip.to1D(x, y)
		}
	}

	// --- Instruction Execution ---
	switch opcode {
	case OP_MOV:
		result := getOperand(modeSrc1)
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_ADD:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2)
		result := val1 + val2
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_SUB:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2)
		result := val1 - val2
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_INC:
		val1 := getOperand(modeSrc1)
		result := val1 + 1
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_DEC:
		val1 := getOperand(modeSrc1)
		result := val1 - 1
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_XOR:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2)
		result := val1 ^ val2
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_AND:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2)
		result := val1 & val2
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_OR:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2)
		result := val1 | val2
		destAddr := getDestAddress(modeDst)
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_SHF:
		val1 := getOperand(modeSrc1)
		val2 := getOperand(modeSrc2) // shift amount
		result := val1
		destAddr := getDestAddress(modeDst)
		if val2 > 0 {
			result = val1 << uint(val2)
		} else {
			result = val1 >> uint(-val2)
		}
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_JMP:
		targetOffset := getOperand(modeSrc1)
		ip.X, ip.Y = resolveAddress(opcodeX, opcodeY, targetOffset)
	case OP_JE:
		val1 := getOperand(modeSrc1)
		if val1 == 0 {
			targetOffset := getOperand(modeSrc2)
			ip.X, ip.Y = resolveAddress(opcodeX, opcodeY, targetOffset)
		}
	case OP_JNE:
		val1 := getOperand(modeSrc1)
		if val1 != 0 {
			targetOffset := getOperand(modeSrc2)
			ip.X, ip.Y = resolveAddress(opcodeX, opcodeY, targetOffset)
		}
	}

		// The instruction finished, we move the "real" IP randomly
		dir := rand.Intn(8)
		dx := directions[dir][0]
		dy := directions[dir][1]
		ip.X += dx
		ip.Y += dy

	// Wrap IP coordinates
	ip.X = ip.wrap(ip.X, ip.SoupDimX)
	ip.Y = ip.wrap(ip.Y, ip.SoupDimY)

	ip.Steps++
}
