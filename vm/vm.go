package vm

import "encoding/binary"

// --- Configuration ---



// --- Instruction Set Opcodes ---
const (
	NOOP byte = iota
	ADD
	SUB
	JUMP_REL_IF_LT_ZERO
	AND
	OR
	XOR
	NOT
	LOAD_VAL_FROM_ADDR
	LOAD_ADDR_FROM_ADDR
	LOAD_VAL_IMMEDIATE
	LOAD_ADDR_IMMEDIATE
	WRITE_VAL_TO_ADDR
	NumOpcodes // This must be the last entry, it counts the number of opcodes
)

var OpcodeNames = [...]string{
	"NOOP",
	"ADD",
	"SUB",
	"JUMP_REL_IF_LT_ZERO",
	"AND",
	"OR",
	"XOR",
	"NOT",
	"LOAD_VAL_FROM_ADDR",
	"LOAD_ADDR_FROM_ADDR",
	"LOAD_VAL_IMMEDIATE",
	"LOAD_ADDR_IMMEDIATE",
	"WRITE_VAL_TO_ADDR",
}

// IP represents an Instruction Pointer, our digital organism.
type IP struct {
	ID                  int
	CurrentPtr          int32 // Current instruction pointer in the soup
	Steps               int64 // Number of steps executed
	Soup                []int8
	ValueRegister       int8
	AddressRegister     int32
	Use32BitAddressing  bool
	UseRelativeAddressing bool
}

// SavableIP defines the data for an IP that can be saved in a snapshot.
type SavableIP struct {
	ID              int
	CurrentPtr      int32
	Steps           int64
	ValueRegister   int8
	AddressRegister int32
	CurrentInstruction int8 // The raw instruction byte at CurrentPtr
}

// Savable returns a serializable representation of the IP.
func (ip *IP) CurrentState() SavableIP {
	// Ensure CurrentPtr is within bounds for accessing Soup
	soupLen := int32(len(ip.Soup))
	wrappedCurrentPtr := (ip.CurrentPtr%soupLen + soupLen) % soupLen

	return SavableIP{
		ID:                 ip.ID,
		CurrentPtr:         ip.CurrentPtr,
		Steps:              ip.Steps,
		ValueRegister:      ip.ValueRegister,
		AddressRegister:    ip.AddressRegister,
		CurrentInstruction: ip.Soup[wrappedCurrentPtr],
	}
}

// NewIP creates a new, minimal instruction pointer.
func NewIP(id int, soup []int8, startPtr int32, use32BitAddressing bool, useRelativeAddressing bool) *IP {
	ip := &IP{
		ID:                  id,
		Soup:                soup,
		CurrentPtr:          startPtr,
		ValueRegister:       0,
		AddressRegister:     0,
		Use32BitAddressing:  use32BitAddressing,
		UseRelativeAddressing: useRelativeAddressing,
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

	// Reads 1 or 4 bytes from a specified memory address (does not advance CurrentPtr).
	readAtAddr := func(addr int32) int32 {
		if ip.Use32BitAddressing {
			var bs = make([]byte, 4)
			for i := 0; i < 4; i++ {
				bs[i] = byte(ip.Soup[wrapAddr(addr+int32(i))])
			}
			return int32(binary.LittleEndian.Uint32(bs))
		} else {
			return int32(ip.Soup[wrapAddr(addr)])
		}
	}

	// Calculates the final address for an operation based on the addressing mode.
	resolveDataAddress := func(opcodeLocation int32) int32 {
		if ip.UseRelativeAddressing {
			return wrapAddr(opcodeLocation + ip.AddressRegister)
		} else {
			return wrapAddr(ip.AddressRegister)
		}
	}

	// --- Instruction Execution ---
	opcodeLocation := ip.CurrentPtr
	opcode := (int32(fetch8())%int32(NumOpcodes) + int32(NumOpcodes)) % int32(NumOpcodes)

	// Pre-calculate the address for any instruction that needs it.
	dataAddr := resolveDataAddress(opcodeLocation)

	switch byte(opcode) {
	case NOOP:
	case ADD:
		ip.ValueRegister += ip.Soup[dataAddr]
	case SUB:
		ip.ValueRegister -= ip.Soup[dataAddr]
	case JUMP_REL_IF_LT_ZERO:
		jumpOffset := ip.AddressRegister
		if ip.ValueRegister < 0 {
			ip.CurrentPtr = opcodeLocation + jumpOffset
		}
	case AND:
		ip.ValueRegister &= ip.Soup[dataAddr]
	case OR:
		ip.ValueRegister |= ip.Soup[dataAddr]
	case XOR:
		ip.ValueRegister ^= ip.Soup[dataAddr]
	case NOT:
		ip.ValueRegister = ^ip.ValueRegister
	case LOAD_VAL_FROM_ADDR:
		ip.ValueRegister = ip.Soup[dataAddr]
	case LOAD_ADDR_FROM_ADDR:
		ip.AddressRegister = readAtAddr(dataAddr)
	case LOAD_VAL_IMMEDIATE:
		ip.ValueRegister = int8(fetchImmediate())
	case LOAD_ADDR_IMMEDIATE:
		ip.AddressRegister = fetchImmediate()
	case WRITE_VAL_TO_ADDR:
		ip.Soup[dataAddr] = ip.ValueRegister
	}
	ip.Steps++
}
