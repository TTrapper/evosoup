# EvoSoup

EvoSoup is an experiment in digital evolution that attempts to simulate life at its most fundamental level. It creates a virtual world where the very definition of an "organism" is not predetermined, but must emerge from the underlying physics of the simulation.

![EvoSoup Screenshot](evoscreenshot.png)

## The Concept

The core of EvoSoup is a shared memory space called the "soup." This soup is initialized with a random sequence of simple instructions. A multitude of "Instruction Pointers" (IPs) concurrently execute this code. These IPs are not the organisms themselves; they are merely pointers, representing the focus of execution.

In this world, there are no predefined boundaries for an organism. Unlike many artificial life projects that explicitly define structures for replication, crossover, and resource management, EvoSoup starts with as little as possible. An "organism" is hypothesized to be an emergent pattern within the soup—a self-sustaining or self-replicating block of code that persists and propagates over time.

The only true resource constraint is the competition for CPU time. Multiple IPs run in parallel without any thread safety, meaning their execution is a chaotic race. This ties the simulation directly to the dynamics of the physical computer, where the ability of a code pattern to be executed and re-executed is its measure of fitness.

Evolution in EvoSoup is therefore a process of discovery, searching for emergent, resilient patterns of computation in a sea of randomness.

## How to Run

1.  **Install Go:** Make sure you have Go installed on your system.
2.  **Run the simulation:** Open a terminal in the project directory and run the following command:

    ```bash
    go run .
    ```

3.  **View the simulation:** Open your web browser and go to `http://localhost:8080`. You will see a real-time visualization of the soup and the IPs. You can also interact with the simulation through the controls on the web page.

### Command-line Options

*   `-load <filename>`: Load a previous simulation state from a snapshot file.
*   `-duration <minutes>`: Run the simulation for a specific number of minutes. If not specified, the simulation will run indefinitely.

## The Frontend

EvoSoup includes a web-based frontend that allows you to visualize and interact with the simulation in real-time. The frontend is served automatically when you run the simulation and can be accessed at `http://localhost:8080`.

The frontend provides:

*   A real-time visualization of the soup's memory.
*   Statistics about the simulation, such as population size and instruction entropy.
*   Controls to pause, resume, and step the simulation.
*   Options to adjust simulation parameters, such as the jump rate and addressing modes.
