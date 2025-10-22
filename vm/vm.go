package vm

import (
	"encoding/binary"
	"math/rand"
)

// --- Rule-based Opcodes & Types ---
// Layout: [aluOp(2) | src1(2) | src2(2) | dest(2)]

// ALU Ops (Bits 7-6)
const (
	ALU_NAND uint8 = 0
	ALU_XOR  uint8 = 1
	ALU_MOV  uint8 = 2
	ALU_JMP  uint8 = 3
)

// OpcodeInfo contains the name and value for a given opcode.
type OpcodeInfo struct {
	Name  string `json:"name"`
	Value uint8  `json:"value"`
}

// GetOpcodes returns a list of all defined opcodes and their values.
func GetOpcodes() []OpcodeInfo {
	return []OpcodeInfo{
		{Name: "NAND", Value: ALU_NAND},
		{Name: "XOR", Value: ALU_XOR},
		{Name: "MOV", Value: ALU_MOV},
		{Name: "JUMP", Value: ALU_JMP},
	}
}

// Source/Dest Types (Bits 5-4, 3-2, 1-0)
const (
	TYPE_IMMEDIATE_VERTICAL  uint8 = 0
	TYPE_ADDR_FROM_IMMEDIATE uint8 = 1
	TYPE_IMMEDIATE_LEFT      uint8 = 2
	TYPE_IMMEDIATE_RIGHT     uint8 = 3
)

// Jump Conditions (Bits 1-0, repurposed from destType)
const (
	JUMP_Z  uint8 = 0 // Jump if == 0
	JUMP_NZ uint8 = 1 // Jump if != 0
	JUMP_GZ uint8 = 2 // Jump if > 0
	JUMP_LZ uint8 = 3 // Jump if < 0
)

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
	locX, locY := ip.X, ip.Y

	// --- Helpers ---
	fetchX, fetchY := ip.X, ip.Y
	advanceFetch := func() {
		fetchX++
		if fetchX >= ip.SoupDimX {
			fetchX = 0
			fetchY++
		}
		fetchY = ip.wrap(fetchY, ip.SoupDimY)
	}

	// The rule is the byte at the IP's current location.
	// We advance the fetch pointer past it immediately.
	ruleDigest := uint8(ip.Soup[ip.to1D(fetchX, fetchY)])
	advanceFetch()

	fetch8 := func() int8 {
		val := ip.Soup[ip.to1D(fetchX, fetchY)]
		advanceFetch()
		return val
	}
	fetch32 := func() int32 {
		b1 := byte(fetch8()); b2 := byte(fetch8()); b3 := byte(fetch8()); b4 := byte(fetch8())
		return int32(binary.BigEndian.Uint32([]byte{b1, b2, b3, b4}))
	}
	fetchImmediate := func() int32 {
		if ip.Use32BitAddressing {
			return fetch32()
		}
		return int32(fetch8())
	}
	resolveAddress := func(baseX, baseY int32, offset int32) int32 {
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
				finalX = offset & 0xFFFF
				finalY = (offset >> 16) & 0xFFFF
			} else {
				finalX = offset & 0xFF
				finalY = (offset >> 8) & 0xFF // Use top 8 bits for Y in absolute 16-bit
			}
		}
		return ip.to1D(finalX, finalY)
	}

	// --- Decode the 8-bit rule ---
	// Clustered Layout: [aluOp(2) | src1(2) | src2(2) | dest(2)]
	aluOp := (ruleDigest >> 6) & 0x03
	srcType := (ruleDigest >> 4) & 0x03
	destType := ruleDigest & 0x03

	// --- Rule Execution ---
	if aluOp == ALU_JMP { // Repurposed aluOp 3 as JUMP
		var offset int32
		switch srcType {
		case TYPE_IMMEDIATE_VERTICAL: offset = fetchImmediate()
		case TYPE_ADDR_FROM_IMMEDIATE:
			addr := resolveAddress(locX, locY, fetchImmediate())
			offset = int32(ip.Soup[addr])
		case TYPE_IMMEDIATE_LEFT: offset = int32(ip.Soup[ip.to1D(locX-1, locY)])
		case TYPE_IMMEDIATE_RIGHT: offset = int32(ip.Soup[ip.to1D(locX+1, locY)])
		}
		jumpIndex := resolveAddress(locX, locY, offset)

		var condVal int8
		switch srcType {
		case TYPE_IMMEDIATE_VERTICAL: condVal = fetch8()
		case TYPE_ADDR_FROM_IMMEDIATE:
			addr := resolveAddress(locX, locY, fetchImmediate())
			condVal = ip.Soup[addr]
		case TYPE_IMMEDIATE_LEFT: condVal = ip.Soup[ip.to1D(locX-1, locY)]
		case TYPE_IMMEDIATE_RIGHT: condVal = ip.Soup[ip.to1D(locX+1, locY)]
		}

		jumpTaken := false
		switch destType { // Repurposed for condition type
		case JUMP_Z: if condVal == 0 { jumpTaken = true }
		case JUMP_NZ: if condVal != 0 { jumpTaken = true }
		case JUMP_GZ: if condVal > 0 { jumpTaken = true }
		case JUMP_LZ: if condVal < 0 { jumpTaken = true }
		}

		if jumpTaken {
			ip.X = jumpIndex % ip.SoupDimX
			ip.Y = jumpIndex / ip.SoupDimX
		}

	} else {
		// --- ALU LOGIC ---
		var src1Val, src2Val int8
		var src1Addr int32 = -1

		switch srcType {
		case TYPE_IMMEDIATE_VERTICAL:
			src1Val = ip.Soup[ip.to1D(locX, locY-1)]
			src2Val = ip.Soup[ip.to1D(locX, locY+1)]
		case TYPE_ADDR_FROM_IMMEDIATE:
			src1Addr = resolveAddress(locX, locY, fetchImmediate())
			src1Val = ip.Soup[src1Addr]
			addr := resolveAddress(locX, locY, fetchImmediate())
			src2Val = ip.Soup[addr]
		case TYPE_IMMEDIATE_LEFT:
			src1Val = ip.Soup[ip.to1D(locX-1, locY)]
			src2Val = ip.Soup[ip.to1D(locX+1, locY)]
		case TYPE_IMMEDIATE_RIGHT:
			src1Val = ip.Soup[ip.to1D(locX+1, locY)]
			src2Val = ip.Soup[ip.to1D(locX-1, locY)]
		}

		var result int8
		switch aluOp {
		case ALU_NAND: result = ^(src1Val & src2Val)
		case ALU_XOR: result = src1Val ^ src2Val
		case ALU_MOV: result = src2Val
		}

		switch destType {
		case 0: // Overwrite self
			ip.Soup[ip.to1D(locX, locY)] = result
		case 1: // Overwrite Left Neighbor
			ip.Soup[ip.to1D(locX-1, locY)] = result
		case 2: // Overwrite Right Neighbor
			ip.Soup[ip.to1D(locX+1, locY)] = result
		case 3: // Write to Address from Src1
			if src1Addr != -1 {
				ip.Soup[src1Addr] = result
			}
		}
	}

	// --- Final Random Movement ---
	direction := rand.Intn(4)
	switch direction {
	case 0: ip.Y-- // Up
	case 1: ip.Y++ // Down
	case 2: ip.X-- // Left
	case 3: ip.X++ // Right
	}

	// Ensure the final IP coordinates are wrapped for the next cycle.
	ip.X = ip.wrap(ip.X, ip.SoupDimX)
	ip.Y = ip.wrap(ip.Y, ip.SoupDimY)

	ip.Steps++
}
