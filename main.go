package main

import (
	"encoding/gob"
	"encoding/json"
	"evolution/vm"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// --- Simulation Constants ---
const (
	SoupSize              = 1024 * 1024
	InitialNumIPs         = 8
	GenerationTimeSeconds = 1
	MinStepsPerGen        = 1   // Minimum steps to be considered "alive"
	SnapshotInterval      = 100 // Save a snapshot every 100 generations
	TargetFPS             = 30  // Target a smooth FPS
)

// SimulationState represents the entire state of the simulation to be saved.
type SimulationState struct {
	Generation int
	Soup       []int32
	IPs        []vm.SavableIP
	NextIPID   int32
	RandSeed   int64 // To be able to resume with the same random sequence
}

var (
	globalJumpInterval int64
	globalTimeElapsed  int64
	globalSteps        int64
)

// runIP is the function that executes in each IP's goroutine.
func runIP(p *vm.IP) {
	for {
		p.Step()
		atomic.AddInt64(&globalSteps, 1)
	}
}

// saveSnapshot function
func saveSnapshot(state SimulationState, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}
	return nil
}

func main() {
	fmt.Println("--- EvoSoup: A Go-based Artificial Life Simulation ---")

	// 1. Create and run the WebSocket hub from our websocket.go file
	hub := NewHub()
	go hub.Run()

	// 2. Start the web server from our websocket.go file
	go StartServer(hub)

	// --- 3. Initialize Simulation ---
	seed := time.Now().UnixNano()
	rand.Seed(seed)

	// Initialize global jump interval
	atomic.StoreInt64(&globalJumpInterval, 1000) // Default to 1000ms (1 second)

	soup := make([]int32, SoupSize)
	for i := range soup {
    soup[i] = rand.Int31()
	}

	var population sync.Map
	var nextIPID int32 = 0

	// Initial population.
	for i := 0; i < InitialNumIPs; i++ {
		startPtr := rand.Int31n(SoupSize)
		newID := atomic.AddInt32(&nextIPID, 1)
		ip := vm.NewIP(int(newID), soup, startPtr)
		population.Store(ip.ID, ip)
	}
	fmt.Printf("Simulation started with %d IPs in a soup of %d instructions. Seed: %d\n", InitialNumIPs, SoupSize, seed)

	// --- Global Jump Timer Goroutine ---
	go func() {
		jumpTicker := time.NewTicker(time.Microsecond) // Check every 1 microsecond
		defer jumpTicker.Stop()

		for range jumpTicker.C {
			atomic.AddInt64(&globalTimeElapsed, 1)
			currentInterval := atomic.LoadInt64(&globalJumpInterval)

			// Only trigger jumps if the interval is positive and the elapsed time is a multiple.
			// This ensures jumps happen at the specified interval.
			if currentInterval > 0 && atomic.LoadInt64(&globalTimeElapsed)%currentInterval == 0 {
				population.Range(func(key, value interface{}) bool {
					ip := value.(*vm.IP)
					// NOTE: This direct assignment to ip.CurrentPtr creates a race condition
					// with the IP's own Step() method. However, given the chaotic nature
					// of the simulation and the performance overhead of mutexes on every
					// IP step, this is deemed acceptable. Occasional skipped or mis-executed
					// instructions due to this race are considered noise.
					ip.CurrentPtr = rand.Int31n(int32(len(soup))) // Force jump
					return true
				})
			}
		}
	}()

	// --- 4. Real-time Visualization Goroutine ---
	go func() {
		ticker := time.NewTicker(time.Second / TargetFPS)
		defer ticker.Stop()

		colorIndices := make([]byte, SoupSize)
		numColors := int32(vm.NumOpcodes) // The number of opcodes/colors you have

		for range ticker.C {
			// Create the color index map from the current soup state
			for i, val := range soup {
				colorIndex := (val%numColors + numColors) % numColors
				colorIndices[i] = byte(colorIndex)
			}

			// Send the raw byte slice to the hub's public broadcast channel.
			hub.Broadcast <- colorIndices
		}
	}()

	// --- Statistics goroutine ---
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var frameIndex = 0
		for range ticker.C {
			// --- Calculate Steps Per Second ---
			currentSteps := atomic.LoadInt64(&globalSteps)
			atomic.StoreInt64(&globalSteps, 0) // Reset for the next second

			// Soup Entropy
			soupCounts := make(map[int32]int)
			frameIndex++
			for _, instr := range soup {
				soupCounts[instr]++
			}
			var soupEntropy float64
			for _, count := range soupCounts {
				p := float64(count) / float64(SoupSize)
				if p > 0 {
					soupEntropy -= p * math.Log2(p)
				}
			}
			stats := GenerationStats{
				Generation:     frameIndex,
				Population:     InitialNumIPs,
				Steps:          0, // We can probably remove this now
				StepsPerSecond: currentSteps,
				Entropy:        soupEntropy,
			}
			jsonData, err := json.Marshal(stats)
			if err != nil {
				log.Printf("error marshalling json: %v", err)
			} else {
				hub.Broadcast <- jsonData
			}
		}
	}()

  // --- Snapshotting goroutine ---
	go func() {
		ticker := time.NewTicker(time.Second * 600)
		var nSaves = 0
		for range ticker.C {
			nSaves++
			var savableIPs []vm.SavableIP
			population.Range(func(key, value interface{}) bool {
				ip := value.(*vm.IP)
				savableIPs = append(savableIPs, ip.Savable())
				return true
			})

			snapshotState := SimulationState{
				Generation: nSaves,
				Soup:       soup,
				IPs:        savableIPs,
				RandSeed:   seed,
			}

			snapshotFilename := "snapshot.gob"
			if err := saveSnapshot(snapshotState, snapshotFilename); err != nil {
				fmt.Printf(" (Error saving snapshot: %v)\n", err)
			} else {
				fmt.Printf(" (Snapshot %d saved to %s)\n", nSaves, snapshotFilename)
			}
		}
	}()

	var wg sync.WaitGroup
	population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		// Reset state and inject tools
		ip.Steps = 0
		wg.Add(1)
		go runIP(ip)
		return true // continue iteration
	})

	// --- 5. Main Simulation Control Loop ---
	for {
		select {
		case newFreq := <-hub.SetJumpInterval:
			// Update the global jump interval based on UI input
			atomic.StoreInt64(&globalJumpInterval, int64(newFreq))
		case <-time.After(time.Second): // Keep the main goroutine alive
			// This case prevents the select from blocking indefinitely if no messages are received
		}
	}
}
