package vm

import "math/rand"

// --- Micro-Architectural Instruction Format ---
// An instruction is a single byte, decoded as a bitfield:
// [ALU_Op(4) | S1_Ptr(1) | S2_Ptr(1) | Destination(2)]
// MSB.............................................LSB

const NumAluBits = 4 // For websocket.go

// ALU Opcodes
const (
	OP_CPY    uint8 = 0
	OP_ADD    uint8 = 1
	OP_SUB    uint8 = 2
	OP_NAND   uint8 = 3
	OP_OR     uint8 = 4
	OP_AND    uint8 = 5
	OP_XOR    uint8 = 6
	OP_NOT_S1 uint8 = 7
	OP_MOV_S1 uint8 = 8
	OP_MOV_S2 uint8 = 9
	OP_INC_S1 uint8 = 10
	OP_DEC_S1 uint8 = 11
	OP_JMP    uint8 = 12 // Unconditional jump
	OP_JZ     uint8 = 13 // Jump if src1Val == 0
	OP_JNZ    uint8 = 14 // Jump if src1Val != 0
	OP_JNEG   uint8 = 15 // Jump if src1Val < 0
)

// OpcodeInfo contains the name and value for a given opcode.
type OpcodeInfo struct {
	Name  string `json:"name"`
	Value uint8  `json:"value"`
}

// GetOpcodes returns a list of all defined opcodes and their values.
func GetOpcodes() []OpcodeInfo {
	return []OpcodeInfo{
		{Name: "CPY", Value: OP_CPY},
		{Name: "ADD", Value: OP_ADD},
		{Name: "SUB", Value: OP_SUB},
		{Name: "NAND", Value: OP_NAND},
		{Name: "OR", Value: OP_OR},
		{Name: "AND", Value: OP_AND},
		{Name: "XOR", Value: OP_XOR},
		{Name: "NOT_S1", Value: OP_NOT_S1},
		{Name: "MOV_S1", Value: OP_MOV_S1},
		{Name: "MOV_S2", Value: OP_MOV_S2},
		{Name: "INC_S1", Value: OP_INC_S1},
		{Name: "DEC_S1", Value: OP_DEC_S1},
		{Name: "JMP", Value: OP_JMP},
		{Name: "JZ", Value: OP_JZ},
		{Name: "JNZ", Value: OP_JNZ},
		{Name: "JNEG", Value: OP_JNEG},
	}
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
	// --- Fetch and Decode ---
	locX, locY := ip.X, ip.Y
	instruction := uint8(ip.Soup[ip.to1D(locX, locY)])

	aluOp := (instruction >> 4) & 0x0F
	s1PtrMode := (instruction >> 3) & 0x01
	s2PtrMode := (instruction >> 2) & 0x01
	destSel := instruction & 0x03
	direction := rand.Intn(4)

	// --- Define Neighbor Locations ---
	northX, northY := locX, locY-1
	eastX, eastY := locX+1, locY

	// --- Helper for address resolution (the "pointer infrastructure") ---
	resolveAddress := func(baseX, baseY int32, offset int32) int32 {
		var finalX, finalY int32
		if ip.UseRelativeAddressing {
			if ip.Use32BitAddressing {
				dx := int32(int16(offset & 0xFFFF))
				dy := int32(int16(offset >> 16))
				finalX = baseX + dx
				finalY = baseY + dy
			} else { // 8-bit relative
				dx := int32(int8(byte(offset)<<4) >> 4) // sign extend low nibble
				dy := int32(int8(byte(offset)) >> 4)      // sign extend high nibble
				finalX = baseX + dx
				finalY = baseY + dy
			}
		} else { // Absolute addressing
			if ip.Use32BitAddressing {
				finalX = offset & 0xFFFF
				finalY = (offset >> 16) & 0xFFFF
			} else {
				finalX = offset & 0xFF
				finalY = (offset >> 8) & 0xFF // Use top 8 bits for Y in absolute 16-bit
			}
		}
		return ip.to1D(finalX, finalY)
	}

	// --- Fetch Operands ---
	var src1Val, src2Val int8
	var src1Addr, src2Addr int32
	// Fetch Src1 from North
	if s1PtrMode == 0 { // Value Mode
		src1Addr = ip.to1D(northX, northY)
		src1Val = ip.Soup[src1Addr]
	} else { // Pointer Mode
		offset := int32(ip.Soup[ip.to1D(northX, northY)])
		src1Addr = resolveAddress(northX, northY, offset)
		src1Val = ip.Soup[src1Addr]
	}

	// Fetch Src2 from East
	if s2PtrMode == 0 { // Value Mode
		src2Addr = ip.to1D(eastX, eastY)
		src2Val = ip.Soup[src2Addr]
	} else { // Pointer Mode
		offset := int32(ip.Soup[ip.to1D(eastX, eastY)])
		src2Addr = resolveAddress(eastX, eastY, offset)
		src2Val = ip.Soup[src2Addr]
	}

	// --- 1. Calculate & Jump Condition Phase ---
	var result int8
	jumpTaken := false

	switch aluOp {
	case OP_CPY:
		result = int8(instruction) // Copy self
	case OP_ADD:
		result = src1Val + src2Val
	case OP_SUB:
		result = src1Val - src2Val
	case OP_NAND:
		result = ^(src1Val & src2Val)
	case OP_OR:
		result = src1Val | src2Val
	case OP_AND:
		result = src1Val & src2Val
	case OP_XOR:
		result = src1Val ^ src2Val
	case OP_NOT_S1:
		result = ^src1Val
	case OP_MOV_S1:
		result = src1Val
	case OP_MOV_S2:
		result = src2Val
	case OP_INC_S1:
		result = src1Val + 1
	case OP_DEC_S1:
		result = src1Val - 1
	case OP_JMP:
		jumpTaken = true
		result = int8(instruction) // Copy self
	case OP_JZ:
		if src1Val == 0 {
			jumpTaken = true
		}
		result = int8(instruction) // Copy self
	case OP_JNZ:
		if src1Val != 0 {
			jumpTaken = true
		}
		result = int8(instruction) // Copy self
	case OP_JNEG:
		if src1Val < 0 {
			jumpTaken = true
		}
		result = int8(instruction) // Copy self
	default:
		// Undefined opcodes are CPYs
		result = int8(instruction) // Copy self
	}

	// --- 2. Write Phase ---
	var destAddr int32
	switch destSel {
	case 0:
		destAddr = src1Addr
	case 1:
		destAddr = src2Addr
	case 2:
		destAddr = ip.to1D(locX, locY)
	case 3:
		// Write to the address pointed to by src2 (the jump address)
		jumpOffset := int32(src2Val)
		destAddr = resolveAddress(locX, locY, jumpOffset)
	}
	ip.Soup[destAddr] = result

	// --- 3. Jump / Move Phase ---
	if jumpTaken {
		jumpOffset := int32(src2Val) // Src2 provides the offset
		jumpIndex := resolveAddress(locX, locY, jumpOffset)
		ip.X = jumpIndex % ip.SoupDimX
		ip.Y = jumpIndex / ip.SoupDimX
	} 
	// Move IP
	switch direction {
	case 0:
		ip.Y-- // North
	case 1:
		ip.X++ // East
	case 2:
		ip.Y++ // South
	case 3:
		ip.X-- // West
	}

	// --- Final Wrap ---
	ip.X = ip.wrap(ip.X, ip.SoupDimX)
	ip.Y = ip.wrap(ip.Y, ip.SoupDimY)
	ip.Steps++
}
