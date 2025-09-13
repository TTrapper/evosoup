package main

import (
	"evolution/vm"
	"flag"
	"fmt"
	"log"
	"time"
)

// --- Simulation Constants ---
const (
	SoupSize              = 1024 * 1024 * 16
	InitialNumIPs         = 8
	TargetFPS             = 30  // Target a smooth FPS

	// StatsAndVisSize represents the portion of the soup used for statistics and
	// visualization, corresponding to 1M instructions.
	StatsAndVisSize = 1024 * 1024

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

func main() {
	fmt.Println("--- EvoSoup: A Go-based Artificial Life Simulation ---")

	// --- Command-line flags ---
	snapshotFilename := flag.String("snapshot", "snapshot.gob", "Filename for the final snapshot.")
	loadFilename := flag.String("load", "", "Load a snapshot file to continue an experiment.")
	experimentDuration := flag.Int("duration", -1, "Time in minutes to run an experiment. Negative values run forever")
	flag.Parse()

	// --- 1. Initialize AppState ---
	appState := NewAppState()

	// --- 2. Create and run the WebSocket hub ---
	hub := NewHub()
	go hub.Run()

	// --- 3. Start the web server ---
	go StartServer(hub, appState)

	// --- 4. Initialize Simulation ---
	if *loadFilename != "" {
		// Load from snapshot
		if err := appState.loadSnapshot(*loadFilename); err != nil {
			log.Fatalf("Failed to load snapshot: %v", err)
		}
		fmt.Printf("Loaded snapshot: %s\n", *loadFilename)

	} else {
		// --- Initialize new simulation ---
		appState.initializeSimulation()
	}

	// --- Global Jump Timer Goroutine ---
	go appState.runCosmicRaySimulator()

	// --- 5. Launch IPs ---
	appState.LaunchIPs()

	// --- 6. Real-time Visualization Goroutine ---
	go appState.RunVisualization(hub)


	go appState.RunStatistics(hub)

	// --- IP State Broadcaster Goroutine ---
	go appState.RunIPStateBroadcaster(hub)

	// --- Snapshotting goroutine ---
	go func() {
		ticker := time.NewTicker(time.Second * 600)
		var nSaves = 0
		for range ticker.C {
			nSaves++
			var snapshotFilenameWithCount = ""
			if *experimentDuration < 0 {
				snapshotFilenameWithCount = fmt.Sprintf("%s_%d.gob", *snapshotFilename, 0)
			} else {
				snapshotFilenameWithCount = fmt.Sprintf("%s_%d.gob", *snapshotFilename, nSaves)
			}
			if err := appState.saveSnapshot(snapshotFilenameWithCount); err != nil {
				fmt.Printf(" (Error saving snapshot: %v)\n", err)
			} else {
				fmt.Printf(" (Snapshot %d saved to %s)\n", nSaves, snapshotFilenameWithCount)
			}
		}
	}()

	// --- 7. Main Simulation Control Loop ---
	if *experimentDuration < 0 {
		fmt.Printf("Negative experiment duration provided, running forever.\n")
		for {
			select {
			case isPaused := <-hub.Pause:
				if isPaused {
					appState.Pause()
				} else {
					appState.Resume()
				}
			case cosmicRayRate := <-hub.SetCosmicRayRate:
				appState.SetCosmicRayRate(cosmicRayRate)
			}
		}
	}
	<-time.After(time.Duration(*experimentDuration) * time.Minute)

	// --- 8. Save final state and entropies ---
	if err := appState.saveSnapshot(*snapshotFilename); err != nil {
		log.Fatalf("failed to save final snapshot: %v", err)
	}

	fmt.Printf("--- Experiment finished. Snapshot and entropies saved.---\n")
}

