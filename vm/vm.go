package vm


// --- Constants ---
const (
	NumRegisters      = 4
	GenomeSnippetSize = 32
)

// --- Instruction Set Opcodes ---
const (
	NOOP byte = iota
	COPY
	ADD
	SUB
	JUMP_REL_IF_LT_ZERO
	WRITE_CONST
	AND
	OR
	XOR
	NOT
	NumOpcodes // This must be the last entry, it counts the number of opcodes
)

var OpcodeNames = [...]string{
	"NOOP",
	"COPY",
	"ADD",
	"SUB",
	"JUMP_REL_IF_LT_ZERO",
	"WRITE_CONST",
	"AND",
	"OR",
	"XOR",
	"NOT",
}

// IP represents an Instruction Pointer, our digital organism.
type IP struct {
	ID            int
	CurrentPtr    int32 // Current instruction pointer in the soup
	Steps         int64 // Number of steps executed
	Soup          []int8
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID         int
	CurrentPtr int32
	Steps      int64
}

// Savable returns a serializable representation of the IP.
func (ip *IP) Savable() SavableIP {
	return SavableIP{
		ID:         ip.ID,
		CurrentPtr: ip.CurrentPtr,
		Steps:      ip.Steps,
	}
}

// NewIP creates a new, minimal instruction pointer.
func NewIP(id int, soup []int8, startPtr int32) *IP {
	ip := &IP{
		ID:            id,
		Soup:          soup,
		CurrentPtr:    startPtr,
	}
	return ip
}

// Step executes a single instruction from the soup.
func (ip *IP) Step() {
	soupLen := int32(len(ip.Soup))

	// --- Helper Functions ---
	// Wraps an address to the valid range of the soup.
	wrapAddr := func(addr int32) int32 {
		return (addr%soupLen + soupLen) % soupLen
	}

	// Reads the value at the CurrentPtr and increments the pointer.
	safeRead := func() int8 {
		val := ip.Soup[wrapAddr(ip.CurrentPtr)]
		ip.CurrentPtr++
		return val
	}

	// Reads an offset value and returns the final, absolute, wrapped address
	// calculated relative to the location of the offset value itself.
	getRelAddr := func() int32 {
		currentAddr := wrapAddr(ip.CurrentPtr)
		offsetVal := safeRead()
		return wrapAddr(currentAddr + int32(offsetVal))
	}

	// --- Instruction Execution ---
	opcodeAddr := wrapAddr(ip.CurrentPtr)
	opcode := (int32(safeRead())%int32(NumOpcodes) + int32(NumOpcodes)) % int32(NumOpcodes)

	switch byte(opcode) {
	case NOOP:
		// PC was already incremented by safeRead.
	case COPY:
		destAddr := getRelAddr()
		srcAddr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[srcAddr]
	case ADD:
		destAddr := getRelAddr()
		src1Addr := getRelAddr()
		src2Addr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[src1Addr] + ip.Soup[src2Addr]
	case SUB:
		destAddr := getRelAddr()
		src1Addr := getRelAddr()
		src2Addr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[src1Addr] - ip.Soup[src2Addr]
	case JUMP_REL_IF_LT_ZERO:
		condAddr := getRelAddr()
		jumpOffset := safeRead()
		if ip.Soup[condAddr] < 0 {
			// The jump is relative to the JUMP instruction's location (opcodeAddr),
			// not the location of the offset operand.
			ip.CurrentPtr = opcodeAddr + int32(jumpOffset)
		}
	case WRITE_CONST:
		destAddr := getRelAddr()
		val := safeRead()
		ip.Soup[destAddr] = val
	case AND:
		destAddr := getRelAddr()
		src1Addr := getRelAddr()
		src2Addr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[src1Addr] & ip.Soup[src2Addr]
	case OR:
		destAddr := getRelAddr()
		src1Addr := getRelAddr()
		src2Addr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[src1Addr] | ip.Soup[src2Addr]
	case XOR:
		destAddr := getRelAddr()
		src1Addr := getRelAddr()
		src2Addr := getRelAddr()
		ip.Soup[destAddr] = ip.Soup[src1Addr] ^ ip.Soup[src2Addr]
	case NOT:
		destAddr := getRelAddr()
		srcAddr := getRelAddr()
		ip.Soup[destAddr] = ^ip.Soup[srcAddr]
	}
	ip.Steps++
}
