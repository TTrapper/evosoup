package vm

import (
	"encoding/binary"
	"math/rand"
)

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
	OP_MUL                       // 208
	OP_MOD                       // 224
)

const RandomMoveChance = 0.05

// OpcodeInfo contains the name and value for a given opcode.
type OpcodeInfo struct {
	Name  string `json:"name"`
	Value uint8  `json:"value"`
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
		{Name: "MUL", Value: OP_MUL},
		{Name: "MOD", Value: OP_MOD},
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
	instr := uint8(ip.Soup[ip.to1D(ip.X, ip.Y)])

	// --- DECODING ---
	opcode := instr & 0xF0
	invertBit := (instr >> 3) & 1
	destModeBits := (instr >> 1) & 0x03
	sourceMode := instr & 0x01 // 0 for Address, 1 for Immediate

	// --- HELPERS ---
	readInt8At := func(x, y int32) int8 {
		return ip.Soup[ip.to1D(x, y)]
	}

	readInt32At := func(x, y int32) int32 {
		b1 := byte(readInt8At(x, y))
		b2 := byte(readInt8At(x, y+1))
		b3 := byte(readInt8At(x, y+2))
		b4 := byte(readInt8At(x, y+3))
		byteSlice := []byte{b1, b2, b3, b4}
		return int32(binary.BigEndian.Uint32(byteSlice))
	}

	resolveAddress := func(baseX, baseY int32, offset int32) (int32, int32) {
		var finalX, finalY int32
		if ip.UseRelativeAddressing {
			if ip.Use32BitAddressing {
				dx := int32(int16(offset & 0xFFFF))
				dy := int32(int16(offset >> 16))
				finalX = baseX + dx
				finalY = baseY + dy
			} else { // 8-bit relative
				dx := int32(int8(byte(offset)))
				dy := int32(0) // 8-bit relative is 1D
				finalX = baseX + dx
				finalY = baseY + dy
			}
		} else { // Absolute addressing
			if ip.Use32BitAddressing {
				finalX = offset & 0xFFFF
				finalY = (offset >> 16) & 0xFFFF
			} else {
				finalX = offset & 0xFF
				finalY = 0
			}
		}
		return ip.wrap(finalX, ip.SoupDimX), ip.wrap(finalY, ip.SoupDimY)
	}

	// --- OPERAND FETCHING ---
	// Locations are fixed: Left for src1, Right for src2
	ptr1X, ptr1Y := ip.X-1, ip.Y
	ptr2X, ptr2Y := ip.X+1, ip.Y

	var src1Val, src2Val int32

	if sourceMode == 1 { // Immediate Mode
		src1Val = int32(readInt8At(ptr1X, ptr1Y))
		src2Val = int32(readInt8At(ptr2X, ptr2Y))
	} else { // Address Mode
		var offset1, offset2 int32
		if ip.Use32BitAddressing {
			offset1 = readInt32At(ptr1X, ptr1Y)
			offset2 = readInt32At(ptr2X, ptr2Y)
		} else {
			offset1 = int32(readInt8At(ptr1X, ptr1Y))
			offset2 = int32(readInt8At(ptr2X, ptr2Y))
		}
		op1X, op1Y := resolveAddress(ip.X, ip.Y, offset1)
		op2X, op2Y := resolveAddress(ip.X, ip.Y, offset2)
		src1Val = int32(readInt8At(op1X, op1Y))
		src2Val = int32(readInt8At(op2X, op2Y))
	}

	// --- DESTINATION ---
	var destAddr int32
	switch destModeBits {
	case 0: // immediate right
		destAddr = ip.to1D(ip.X+1, ip.Y)
	case 1: // immediate left
		destAddr = ip.to1D(ip.X-1, ip.Y)
	case 2: // overwrite self
		destAddr = ip.to1D(ip.X, ip.Y)
	case 3: // address from pointer (read from NE)
		destPtrX, destPtrY := ip.X+1, ip.Y-1 // North-East

		var destOffset int32
		if ip.Use32BitAddressing {
			destOffset = readInt32At(destPtrX, destPtrY)
		} else {
			destOffset = int32(readInt8At(destPtrX, destPtrY))
		}

		finalDestX, finalDestY := resolveAddress(ip.X, ip.Y, destOffset)
		destAddr = ip.to1D(finalDestX, finalDestY)
	}

	// --- EXECUTION ---
	jumped := false
	switch opcode {
	case OP_MOV:
		result := src1Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_ADD:
		result := src1Val + src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_SUB:
		result := src1Val - src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_INC:
		result := src1Val + 1
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_DEC:
		result := src1Val - 1
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_XOR:
		result := src1Val ^ src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_AND:
		result := src1Val & src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_OR:
		result := src1Val | src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_SHF:
		result := src1Val
		shift := src2Val
		if shift > 0 {
			result = src1Val << uint(shift)
		} else {
			result = src1Val >> uint(-shift)
		}
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_MUL:
		result := src1Val * src2Val
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_MOD:
		var result int32
		if src2Val == 0 {
			result = 0 // Safe modulo
		} else {
			result = src1Val % src2Val
		}
		if invertBit == 1 {
			result = ^result
		}
		ip.Soup[destAddr] = int8(result)
	case OP_JMP:
		targetOffset := src1Val
		ip.X, ip.Y = resolveAddress(ip.X, ip.Y, targetOffset)
		jumped = true
	case OP_JE:
		if src1Val == 0 {
			targetOffset := src2Val
			ip.X, ip.Y = resolveAddress(ip.X, ip.Y, targetOffset)
			jumped = true
		}
	case OP_JNE:
		if src1Val != 0 {
			targetOffset := src2Val
			ip.X, ip.Y = resolveAddress(ip.X, ip.Y, targetOffset)
			jumped = true
		}
	}

	// --- MOVEMENT ---
	var nextX, nextY int32
	if jumped {
		nextX, nextY = ip.X, ip.Y
	} else {
		nextX, nextY = ip.X, ip.Y
	}

	// Unconditional Random Move
	dir := rand.Intn(8)
	dx := directions[dir][0]
	dy := directions[dir][1]
	ip.X = nextX + int32(dx)
	ip.Y = nextY + int32(dy)

	// Wrap IP coordinates
	ip.X = ip.wrap(ip.X, ip.SoupDimX)
	ip.Y = ip.wrap(ip.Y, ip.SoupDimY)

	ip.Steps++
}
