package main

import (
	"encoding/gob"
	"encoding/json"
	"evolution/vm"
	"flag"
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
	SoupSize              = 1024 * 1024 * 8
	InitialNumIPs         = 8
	TargetFPS             = 30  // Target a smooth FPS

	// StatsAndVisSize represents the portion of the soup used for statistics and
	// visualization, corresponding to 1M instructions.
	StatsAndVisSize = 1024 * 1024

	// ExperimentDuration is the total time each experiment will run.
	ExperimentDuration = 30 * time.Minute
)

// --- Structs ---

// GenerationStats holds statistics for a single generation.
type GenerationStats struct {
	Generation     int     `json:"Generation"`
	Population     int     `json:"Population"`
	StepsPerSecond int64   `json:"StepsPerSecond"`
	Entropy        float64 `json:"Entropy"`
}

// SimulationState represents the entire state of the simulation to be saved.
type SimulationState struct {
	Generation int
	Soup       []int8
	IPs        []vm.SavableIP
	NextIPID   int32
	RandSeed   int64 // To be able to resume with the same random sequence
}

var (
	globalJumpInterval int64
	globalTimeElapsed  int64
	globalSteps        int64
)

// --- Core Simulation Logic ---

// runIP is the function that executes in each IP's goroutine.
func runIP(p *vm.IP) {
	for {
		p.Step()
	}
}

// --- Utility Functions ---

// loadSnapshot loads a simulation state from a .gob file.
func loadSnapshot(filename string) (SimulationState, error) {
	var state SimulationState
	file, err := os.Open(filename)
	if err != nil {
		return state, fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return state, fmt.Errorf("failed to decode snapshot: %w", err)
	}
	return state, nil
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

// saveEntropies saves the entropy history to a CSV file.
func saveEntropies(entropies []float64, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create entropy file: %w", err)
	}
	defer file.Close()

	// Write header
	_, err = file.WriteString("Generation,Entropy\n")
	if err != nil {
		return fmt.Errorf("failed to write header to entropy file: %w", err)
	}

	// Write entropies
	for i, entropy := range entropies {
		_, err = file.WriteString(fmt.Sprintf("%d,%f\n", i+1, entropy))
		if err != nil {
			return fmt.Errorf("failed to write to entropy file: %w", err)
		}
	}

	return nil
}

func main() {
	fmt.Println("--- EvoSoup: A Go-based Artificial Life Simulation ---")

	// --- Command-line flags ---
	snapshotFilename := flag.String("snapshot", "snapshot.gob", "Filename for the final snapshot.")
	entropyFilename := flag.String("entropy", "entropies.csv", "Filename for the entropy history.")
	loadFilename := flag.String("load", "", "Load a snapshot file to continue an experiment.")
	flag.Parse()

	// --- 1. Create and run the WebSocket hub ---
	hub := NewHub()
	go hub.Run()

	// --- 2. Start the web server ---
	go StartServer(hub)

	// --- 3. Initialize Simulation ---
	var state SimulationState
	var soup []int8
	var population sync.Map
	var nextIPID int32
	var seed int64

	if *loadFilename != "" {
		// Load from snapshot
		var err error
		state, err = loadSnapshot(*loadFilename)
		if err != nil {
			log.Fatalf("Failed to load snapshot: %v", err)
		}
		soup = state.Soup
		nextIPID = state.NextIPID
		seed = state.RandSeed
		rand.Seed(seed)

		for _, savableIP := range state.IPs {
			ip := vm.NewIP(savableIP.ID, soup, savableIP.CurrentPtr)
			population.Store(ip.ID, ip)
		}
		fmt.Printf("Loaded snapshot: %s\n", *loadFilename)

	} else {
		// --- Initialize new simulation ---
		seed = time.Now().UnixNano()
		rand.Seed(seed)

		// Initialize global jump interval
		atomic.StoreInt64(&globalJumpInterval, 1) // Default to 1 microsecond

		soup = make([]int8, SoupSize)
		for i := range soup {
			soup[i] = int8(rand.Intn(256) - 128)
		}

		// Initial population.
		for i := 0; i < InitialNumIPs; i++ {
			startPtr := rand.Int31n(SoupSize)
			newID := atomic.AddInt32(&nextIPID, 1)
			ip := vm.NewIP(int(newID), soup, startPtr)
			population.Store(ip.ID, ip)
		}
		fmt.Printf("Simulation started with %d IPs in a soup of %d instructions. Seed: %d\n", InitialNumIPs, SoupSize, seed)
	}

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

		colorIndices := make([]byte, StatsAndVisSize)
		numColors := int32(vm.NumOpcodes) // The number of opcodes/colors you have

		for range ticker.C {
			// Create the color index map from the current soup state
			for i, val := range soup[:StatsAndVisSize] {
				colorIndex := (int32(val)%numColors + numColors) % numColors
				colorIndices[i] = byte(colorIndex)
			}

			// Send the raw byte slice to the hub's public broadcast channel.
			hub.Broadcast <- colorIndices
		}
	}()

	// --- Statistics goroutine ---
	var entropies []float64
	statsDone := make(chan struct{})
	go func() {
		defer close(statsDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var frameIndex = 0
		for {
			select {
			case <-ticker.C:
				// --- Calculate Steps Per Second ---
				var totalSteps int64
				population.Range(func(key, value interface{}) bool {
					ip := value.(*vm.IP)
					totalSteps += ip.Steps
					return true
				})
				var stepsPerSecond = 1000000 * totalSteps / atomic.LoadInt64(&globalTimeElapsed)

				// Soup Entropy
				soupCounts := make(map[int32]int)
				frameIndex++
				for _, instr := range soup[:StatsAndVisSize] {
					soupCounts[int32(instr)]++
				}
				var soupEntropy float64
				for _, count := range soupCounts {
					p := float64(count) / float64(StatsAndVisSize)
					if p > 0 {
						soupEntropy -= p * math.Log2(p)
					}
				}
				entropies = append(entropies, soupEntropy)
				stats := GenerationStats{
					Generation:     frameIndex,
					Population:     InitialNumIPs, // This is a placeholder
					StepsPerSecond: stepsPerSecond,
					Entropy:        soupEntropy,
				}
				jsonData, err := json.Marshal(stats)
				if err != nil {
					log.Printf("error marshalling json: %v", err)
				} else {
					hub.Broadcast <- jsonData
				}
			case <-time.After(ExperimentDuration):
				return
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

			snapshotFilenameWithCount := fmt.Sprintf("%s_%d.gob", *snapshotFilename, nSaves)
			if err := saveSnapshot(snapshotState, snapshotFilenameWithCount); err != nil {
				fmt.Printf(" (Error saving snapshot: %v)\n", err)
			} else {
				fmt.Printf(" (Snapshot %d saved to %s)\n", nSaves, snapshotFilenameWithCount)
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
	<-time.After(ExperimentDuration)

	// --- 6. Save final state and entropies ---
	var savableIPs []vm.SavableIP
	population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		savableIPs = append(savableIPs, ip.Savable())
		return true
	})

	snapshotState := SimulationState{
		Generation: 0, // Final state
		Soup:       soup,
		IPs:        savableIPs,
		RandSeed:   seed,
	}

	if err := saveSnapshot(snapshotState, *snapshotFilename); err != nil {
		log.Fatalf("failed to save final snapshot: %v", err)
	}

	if err := saveEntropies(entropies, *entropyFilename); err != nil {
		log.Fatalf("failed to save entropies: %w", err)
	}

	fmt.Printf("--- Experiment finished. Snapshot and entropies saved.---\n")
}
