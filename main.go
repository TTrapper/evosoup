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

// runIP is the function that executes in each IP's goroutine.
func runIP(p *vm.IP) {
	for {
		p.Step()
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
				Generation: frameIndex,
				Population: InitialNumIPs,
				Steps:      0,
				Entropy:    soupEntropy,
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

	wg.Wait()
}
