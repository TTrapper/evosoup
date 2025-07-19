# EvoSoup

EvoSoup is an experiment in digital evolution where the selective pressures are the fundamental "physics" of the computer itself, not abstract biological metaphors. We have a virtual machine with random code and we want to see what patterns emerge. Organisms are completely implicit and virtual now.

## Core Concepts

*   **The Soup**: A large, shared array of 32-bit integers (`[]int32`). This is the environment. It contains the code that the IPs execute. Any value in the soup can be interpreted as an instruction's opcode or an argument.
*   **Instruction Pointers (IPs)**: These are the digital organisms. Each IP has its own set of registers and a pointer to its current location in the soup. They execute instructions from the soup, one step at a time.
*   **Concurrency**: Each IP runs in its own lightweight goroutine, allowing for massive parallelism and chaotic, non-deterministic interactions within the soup.
*   **Evolution**: There is no explicit fitness function. We still intend to tie simulation to the physical machine by providing the option to jump points after a time interval rather than number of steps.

## How to Run

Simply run the `main.go` file:

```bash
go run main.go
```

## Configuration

The primary simulation parameters are defined as constants in `main.go`:

*   `SoupSize`: The size of the shared memory array.
*   `InitialNumIPs`: The number of IPs to start with.
*   `TargetFPS`: The target frames per second for the visualization.

## The Virtual Machine (VM)

The simulation uses a simple custom VM to execute the IP's code.

*   **Registers**: Each IP has `4` general-purpose 32-bit registers.
*   **Instruction Format**: Instructions are simple. The first `int32` at the IP's current pointer is the opcode. Subsequent `int32` values are the arguments (e.g., register indices, literal values).

### Instruction Set

The opcode is determined by `value % NumOpcodes`.

| Opcode             | Arguments                               | Description                                                                                                                            |
| ------------------ | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `NOOP`             | 0                                       | Does nothing.                                                                                                                          |
| `SET`              | `reg_dest`, `value`                     | Sets `reg_dest` to the literal `value`.                                                                                                |
| `COPY`             | `reg_dest`, `reg_src`                   | Copies the value from `reg_src` to `reg_dest`.                                                                                         |
| `ADD`              | `reg_dest`, `reg_src1`, `reg_src2`      | Adds the values in `reg_src1` and `reg_src2` and stores the result in `reg_dest`.                                                      |
| `SUB`              | `reg_dest`, `reg_src1`, `reg_src2`      | Subtracts the value in `reg_src2` from `reg_src1` and stores the result in `reg_dest`.                                                 |
| `JUMP_REL_IF_ZERO` | `reg_cond`, `reg_offset`                | If the value in `reg_cond` is 0, jumps the instruction pointer by the amount in `reg_offset` (relative to the `JUMP` instruction).      |
| `READ_REL`         | `reg_dest`, `reg_offset`                | Reads a value from the soup at `current_ptr + reg_offset` and stores it in `reg_dest`. The address is wrapped to prevent out-of-bounds. |
| `WRITE_REL`        | `reg_src`, `reg_offset`                 | Writes the value from `reg_src` into the soup at `current_ptr + reg_offset`. The address is wrapped to prevent out-of-bounds.          |