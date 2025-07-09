# EvoSoup

EvoSoup is an experiment in digital evolution where the selective pressures are the fundamental "physics" of the computer itself, not abstract biological metaphors. Unlike many ALife projects that use predefined fitness functions, EvoSoup's organisms compete directly for raw CPU time and space within a chaotic, shared memory environment. The core idea is to discover what survival and replication strategies emerge purely from the constraints of concurrency and resource contention.

## Core Concepts

*   **The Soup**: A large, shared array of 32-bit integers (`[]int32`). This is the environment. It contains the code that the IPs execute. Any value in the soup can be interpreted as an instruction's opcode or an argument.
*   **Instruction Pointers (IPs)**: These are the digital organisms. Each IP has its own set of registers and a pointer to its current location in the soup. They execute instructions from the soup, one step at a time.
*   **Generations**: The simulation runs in discrete time blocks called generations. Each generation lasts for a fixed duration (e.g., 1 second).
*   **Concurrency**: Each IP runs in its own lightweight goroutine, allowing for massive parallelism and chaotic, non-deterministic interactions within the soup.
*   **Fitness & Evolution**:
    *   There is no explicit fitness function.
    *   An IP "survives" a generation if it executes a minimum number of instructions (`MinStepsPerGen`).
    *   Survivors are carried over to the next generation.
    *   If the entire population fails to meet this threshold, an "extinction event" occurs, and the simulation is re-seeded with a new random population.
*   **Replication**: IPs can execute a `SPAWN` instruction. This sends a request to a central population manager, which creates a new child IP at a location specified by the parent. The success of a spawn depends on the population manager's channel not being full, creating a natural cap on the rate of reproduction.

## How to Run

Simply run the `main.go` file:

```bash
go run main.go
```

## Configuration

The primary simulation parameters are defined as constants in `main.go`:

*   `SoupSize`: The size of the shared memory array.
*   `InitialNumIPs`: The number of IPs to start with at generation 0 (and after an extinction event).
*   `GenerationTimeSeconds`: The duration of each generation in seconds.
*   `MinStepsPerGen`: The minimum number of instructions an IP must execute to survive a generation.

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
| `SPAWN`            | `reg_offset`                            | Sends a request to create a child IP whose instruction pointer starts at `current_ptr + reg_offset`.                                   |