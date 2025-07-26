package main

import (
	"encoding/gob"
	"encoding/json"
	"log"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"evolution/vm"
)

// AppState holds the entire application's state, including simulation and UI settings.
type AppState struct {
	// Simulation state
	soup        []int8
	population  sync.Map
	nextIPID    int32
	randSeed    int64
	generation  int
	entropies   []float64
	timeElapsed int64 // In microseconds

	// Control state
	jumpInterval        int64
	ipCount             int32
	paused              int32 // Atomic boolean: 0 for running, 1 for paused
	singleStep          int32 // Atomic boolean: 0 for normal, 1 for single-step
	Use32BitAddressing  bool
	UseRelativeAddressing bool

	// Visualization state
	viewStartIndex int
	viewEndIndex   int
}

// NewAppState initializes a new simulation state.
func NewAppState() *AppState {
	return &AppState{
		soup:                  make([]int8, SoupSize),
		jumpInterval:          1, // Default jump interval
		viewStartIndex:        0,
		viewEndIndex:          StatsAndVisSize,
		Use32BitAddressing:    false, // Default from vm/vm.go
		UseRelativeAddressing: true,  // Default from vm/vm.go
	}
}

// loadSnapshot loads a simulation state from a .gob file.
func (s *AppState) loadSnapshot(filename string) error {
	var state SimulationState
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	s.generation = state.Generation
	s.soup = state.Soup
	s.nextIPID = state.NextIPID
	s.randSeed = state.RandSeed
	rand.Seed(s.randSeed)

	// Clear existing population before loading new ones
	s.population.Range(func(key, value interface{}) bool {
		s.population.Delete(key)
		return true
	})
	atomic.StoreInt32(&s.ipCount, 0)

	for _, savableIP := range state.IPs {
		ip := vm.NewIP(savableIP.ID, s.soup, savableIP.CurrentPtr, s.Use32BitAddressing, s.UseRelativeAddressing)
		s.population.Store(ip.ID, ip)
		atomic.AddInt32(&s.ipCount, 1)
	}

	return nil
}

// saveSnapshot saves the current simulation state to a .gob file.
func (s *AppState) saveSnapshot(filename string) error {
	var savableIPs []vm.SavableIP
	s.population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		savableIPs = append(savableIPs, ip.Savable())
		return true
	})

	snapshotState := SimulationState{
		Generation: s.generation,
		Soup:       s.soup,
		IPs:        savableIPs,
		RandSeed:   s.randSeed,
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(snapshotState); err != nil {
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}
	return nil
}

// initializeSimulation sets up a new simulation with random values.
func (s *AppState) initializeSimulation() {
	s.randSeed = time.Now().UnixNano()
	rand.Seed(s.randSeed)

	for i := range s.soup {
		s.soup[i] = int8(rand.Intn(256) - 128)
	}

	atomic.StoreInt32(&s.ipCount, 0)
	for i := 0; i < InitialNumIPs; i++ {
		startPtr := rand.Int31n(SoupSize)
		newID := atomic.AddInt32(&s.nextIPID, 1)
		ip := vm.NewIP(int(newID), s.soup, startPtr, s.Use32BitAddressing, s.UseRelativeAddressing)
		s.population.Store(ip.ID, ip)
		atomic.AddInt32(&s.ipCount, 1)
	}
	fmt.Printf("Simulation started with %d IPs in a soup of %d instructions. Seed: %d", InitialNumIPs, SoupSize, s.randSeed)
}

// runIP is the execution loop for a single IP.
func (s *AppState) runIP(p *vm.IP) {
	for {
		// Check if simulation is paused
		for atomic.LoadInt32(&s.paused) == 1 && atomic.LoadInt32(&s.singleStep) == 0 {
			time.Sleep(100 * time.Millisecond) // Prevent busy-waiting
		}

		p.Step()

		// If in single-step mode, pause after one step
		if atomic.LoadInt32(&s.singleStep) == 1 {
			atomic.StoreInt32(&s.paused, 1)
			atomic.StoreInt32(&s.singleStep, 0) // Consume the single step
		}
	}
}

// LaunchIPs starts the execution goroutines for all IPs in the population.
func (s *AppState) LaunchIPs() {
	s.population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		go s.runIP(ip)
		return true
	})
}

// SetJumpInterval sets the interval for random jumps.
func (s *AppState) SetJumpInterval(interval int64) {
	atomic.StoreInt64(&s.jumpInterval, interval)
}

// runJumpTimer manages the global jump mechanism.
func (s *AppState) runJumpTimer() {
	jumpTicker := time.NewTicker(time.Microsecond)
	defer jumpTicker.Stop()

	for range jumpTicker.C {
		if atomic.LoadInt32(&s.paused) == 1 {
			time.Sleep(100 * time.Millisecond) // Prevent busy-waiting
			continue
		}
		atomic.AddInt64(&s.timeElapsed, 1)
		currentInterval := atomic.LoadInt64(&s.jumpInterval)

		if currentInterval > 0 && atomic.LoadInt64(&s.timeElapsed)%currentInterval == 0 {
			s.population.Range(func(key, value interface{}) bool {
				ip := value.(*vm.IP)
				ip.ValueRegister = int8(rand.Intn(256) - 128)
				ip.AddressRegister = int32(rand.Intn(256) - 128)
				return true
			})
		}
	}
}

// Pause sets the paused state of the simulation.
func (s *AppState) Pause() {
	atomic.StoreInt32(&s.paused, 1)
}

// Resume sets the paused state of the simulation to false.
func (s *AppState) Resume() {
	atomic.StoreInt32(&s.paused, 0)
}

// Step advances the simulation by one step if it is paused.
func (s *AppState) Step() {
	if atomic.LoadInt32(&s.paused) == 1 {
		atomic.StoreInt32(&s.singleStep, 1)
		atomic.StoreInt32(&s.paused, 0) // Allow one step to execute
	}
}

// SetViewStartIndex sets the starting index for the visualization.
func (s *AppState) SetViewStartIndex(index int) {
	if index < 0 {
		index = 0
	}
	if index >= SoupSize {
		index = SoupSize - StatsAndVisSize // Ensure it doesn't go out of bounds
	}
	s.viewStartIndex = index
	s.viewEndIndex = s.viewStartIndex + StatsAndVisSize
}

// SetRelativeAddressing sets the relative addressing mode.
func (s *AppState) SetRelativeAddressing(enabled bool) {
	s.UseRelativeAddressing = enabled
	s.population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		ip.UseRelativeAddressing = enabled
		return true
	})
}

// Set32BitAddressing sets the 32-bit addressing mode.
func (s *AppState) Set32BitAddressing(enabled bool) {
	s.Use32BitAddressing = enabled
	s.population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		ip.Use32BitAddressing = enabled
		return true
	})
}

// RunVisualization manages the real-time visualization.
func (s *AppState) RunVisualization(hub *Hub) {
	ticker := time.NewTicker(time.Second / TargetFPS)
	defer ticker.Stop()

	colorIndices := make([]byte, StatsAndVisSize) // Use StatsAndVisSize for the visualization
	numColors := int32(vm.NumOpcodes)

	for range ticker.C {
		// Ensure viewStartIndex and viewEndIndex are within bounds
		currentViewStartIndex := s.viewStartIndex
		currentViewEndIndex := s.viewEndIndex
		if currentViewEndIndex > SoupSize {
			currentViewEndIndex = SoupSize
			currentViewStartIndex = SoupSize - StatsAndVisSize
		}
		if currentViewStartIndex < 0 {
			currentViewStartIndex = 0
			currentViewEndIndex = StatsAndVisSize
		}

		// Create the color index map from the current soup state
		for i, val := range s.soup[currentViewStartIndex:currentViewEndIndex] {
			colorIndex := (int32(val)%numColors + numColors) % numColors
			colorIndices[i] = byte(colorIndex)
		}

		// Send the raw byte slice to the hub's public broadcast channel.
		hub.Broadcast <- colorIndices
	}
}

func (s *AppState) RunStatistics(hub *Hub) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var frameIndex = 0
	for {
		if atomic.LoadInt32(&s.paused) == 1 {
			time.Sleep(100 * time.Millisecond) // Prevent busy-waiting
			continue
		}
		select {
		case <-ticker.C:
			// --- Calculate Steps Per Second ---
			var totalSteps int64
			s.population.Range(func(key, value interface{}) bool {
				ip := value.(*vm.IP)
				totalSteps += ip.Steps
				return true
			})
			var stepsPerSecond = 1000000 * totalSteps / atomic.LoadInt64(&s.timeElapsed)

			// Soup Entropy
			soupCounts := make(map[int32]int)
			frameIndex++
			for _, instr := range s.soup[:StatsAndVisSize] {
				soupCounts[int32(instr)]++
			}
			var soupEntropy float64
			for _, count := range soupCounts {
				p := float64(count) / float64(StatsAndVisSize)
				if p > 0 {
					soupEntropy -= p * math.Log2(p)
				}
			}
			s.entropies = append(s.entropies, soupEntropy)
			stats := GenerationStats{
				Generation:     frameIndex,
				Population:     int(atomic.LoadInt32(&s.ipCount)),
				StepsPerSecond: stepsPerSecond,
				Entropy:        soupEntropy,
			}
			jsonData, err := json.Marshal(stats)
			if err != nil {
				log.Printf("error marshalling json: %v", err)
			} else {
				hub.Broadcast <- jsonData
			}
		}
	}
}
