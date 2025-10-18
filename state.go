package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
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
	soup                    []int8
	population              sync.Map
	nextIPID                int32
	randSeed                int64
	generation              int
	entropies               []float64
	timeElapsed             int64 // In microseconds
	cosmicRayRate           uint64

	// Control state
	ipCount             int32
	paused              int32 // Atomic boolean: 0 for running, 1 for paused
	Use32BitAddressing  bool
	UseRelativeAddressing bool

	// Goroutine management
	ipStopChan chan struct{}
	ipWg       sync.WaitGroup

	// Visualization state
	viewStartIndex int
	viewEndIndex   int
	visRequestChan chan struct{} // For on-demand visualization updates

	// IP Tracking
	trackingEnabled bool
	ipStateChan     chan vm.SavableIP
}

// NewAppState initializes a new simulation state.
func NewAppState() *AppState {
	s := &AppState{
		soup:                  make([]int8, SoupSize),
		viewStartIndex:        0,
		viewEndIndex:          StatsAndVisSize,
		Use32BitAddressing:    false,
		UseRelativeAddressing: true,
		trackingEnabled:       false,
		ipStateChan:           make(chan vm.SavableIP, 100), // Buffered channel for IP state updates
		ipStopChan:            make(chan struct{}),
		visRequestChan:        make(chan struct{}, 1),
	}
	// Set a default cosmic ray rate.
	atomic.StoreUint64(&s.cosmicRayRate, math.Float64bits(0.001))
	return s
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
		ip := vm.NewIP(savableIP.ID, s.soup, savableIP.X, savableIP.Y, SoupDimX, s.Use32BitAddressing, s.UseRelativeAddressing)
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
		savableIPs = append(savableIPs, ip.CurrentState())
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
		startX := rand.Int31n(SoupDimX)
		startY := rand.Int31n(SoupDimY)
		newID := atomic.AddInt32(&s.nextIPID, 1)
		ip := vm.NewIP(int(newID), s.soup, startX, startY, SoupDimX, s.Use32BitAddressing, s.UseRelativeAddressing)
		s.population.Store(ip.ID, ip)
		atomic.AddInt32(&s.ipCount, 1)
	}
	fmt.Printf("Simulation started with %d IPs in a soup of %d instructions. Seed: %d", InitialNumIPs, SoupSize, s.randSeed)
}

// runIP is the execution loop for a single IP.
func (s *AppState) runIP(p *vm.IP) {
	defer s.ipWg.Done()
	for {
		select {
		case <-s.ipStopChan:
			return // Exit goroutine when stop signal is received
		default:
			p.Step()
		}
	}
}

// RunIPStateBroadcaster sends the tracked IP's state to the UI.
func (s *AppState) RunIPStateBroadcaster(hub *Hub) {
	for ipState := range s.ipStateChan {
		log.Printf("Broadcasting IP state for IP %d", ipState.ID)
		jsonData, err := json.Marshal(ipState)
		if err != nil {
			log.Printf("error marshalling IP state: %v", err)
		} else {
			hub.Broadcast <- jsonData
		}
	}
}

// LaunchIPs starts the execution goroutines for all IPs in the population.
func (s *AppState) LaunchIPs() {
	s.population.Range(func(key, value interface{}) bool {
		ip := value.(*vm.IP)
		s.ipWg.Add(1)
		go s.runIP(ip)
		return true
	})
}

// SetCosmicRayRate sets the rate for cosmic rays.
func (s *AppState) SetCosmicRayRate(rate float64) {
	atomic.StoreUint64(&s.cosmicRayRate, math.Float64bits(rate))
}

// runCosmicRaySimulator picks a random index in the Soup and flips a bit.
func (s *AppState) runCosmicRaySimulator() {

	for {
		if atomic.LoadInt32(&s.paused) == 1 {
			time.Sleep(100 * time.Millisecond) // Prevent busy-waiting
			continue
		}
		currentRateBits := atomic.LoadUint64(&s.cosmicRayRate)
		p := math.Float64frombits(currentRateBits)
		if p > 0 && rand.Float64() < p {
				// Pick a random index and flip a random bit.
				index := rand.Intn(len(s.soup))
				bit := uint(rand.Intn(8))
				s.soup[index] ^= (1 << bit)
			}
	}
}

// Pause sets the paused state of the simulation.
func (s *AppState) Pause() {
	log.Println("Pausing simulation")
	if atomic.CompareAndSwapInt32(&s.paused, 0, 1) { // If was running (0), set to paused (1)
		close(s.ipStopChan) // Signal all runIP goroutines to stop
		s.ipWg.Wait()       // Wait for all runIP goroutines to finish

		// Request a visualization update to show the final state.
		select {
		case s.visRequestChan <- struct{}{}:
		default:
		}
	} else {
		log.Println("Simulation is already paused.")
	}
}

// Resume sets the paused state of the simulation to false.
func (s *AppState) Resume() {
	log.Println("Resuming simulation")
	if atomic.CompareAndSwapInt32(&s.paused, 1, 0) { // If was paused (1), set to running (0)
		s.ipStopChan = make(chan struct{}) // Re-initialize the channel
		s.LaunchIPs()                     // Restart IP goroutines
	} else {
		log.Println("Simulation is already running.")
	}
}

// Step advances the simulation by one step if it is paused.
func (s *AppState) Step() {
	log.Println("Stepping simulation")
	if atomic.LoadInt32(&s.paused) == 1 {
		if s.trackingEnabled {
			if val, ok := s.population.Load(1); ok { // Always track IP with ID 1
				ip := val.(*vm.IP)
				ip.Step() // Execute one step for the tracked IP
				log.Printf("Stepped tracked IP %d. Sending state.", ip.ID)
				s.ipStateChan <- ip.CurrentState() // Send its state
			} else {
				log.Printf("Tracked IP with ID 1 not found.")
			}
		} else {
			// If tracking is not enabled, step all IPs
			s.population.Range(func(key, value interface{}) bool {
				ip := value.(*vm.IP)
				ip.Step()
				return true
			})
			log.Println("Stepped all IPs.")
		}
		// Keep the simulation paused after the step
		atomic.StoreInt32(&s.paused, 1)
	} else {
		log.Println("Step command received, but simulation is not paused.")
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

	// Request a visualization update, especially important when paused.
	select {
	case s.visRequestChan <- struct{}{}:
	default:
	}
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

// SetTrackingEnabled sets the tracking state of the first IP.
func (s *AppState) SetTrackingEnabled(enabled bool) {
	s.trackingEnabled = enabled
	if enabled {
		log.Println("IP tracking enabled.")
		// When tracking is enabled, immediately send the current state of the tracked IP.
		if val, ok := s.population.Load(1); ok { // Always track IP with ID 1
			ip := val.(*vm.IP)
			log.Printf("Sending initial state for tracked IP %d.", ip.ID)
			s.ipStateChan <- ip.CurrentState()
		} else {
			log.Printf("Tracked IP with ID 1 not found for initial state send.")
		}
	} else {
		log.Println("IP tracking disabled.")
	}
}

// SetIPPtr sets the X, Y of a specific IP from a 1D pointer.
func (s *AppState) SetIPPtr(id int, ptr int32) {
	if val, ok := s.population.Load(id); ok {
		ip := val.(*vm.IP)
		ip.X = ptr % SoupDimX
		ip.Y = ptr / SoupDimX
		log.Printf("Set IP %d position to (%d, %d)", id, ip.X, ip.Y)
	} else {
		log.Printf("IP with ID %d not found to set pointer.", id)
	}
}

// RunVisualization manages the real-time visualization.
func (s *AppState) RunVisualization(hub *Hub) {
	ticker := time.NewTicker(time.Second / TargetFPS)
	defer ticker.Stop()

	currentIndices := make([]byte, StatsAndVisSize) // Allocate once

	sendFrame := func() {
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
			currentIndices[i] = byte(val)
		}

		// Send the raw byte slice to the hub's public broadcast channel.
		hub.Broadcast <- currentIndices
	}

	for {
		select {
		case <-ticker.C:
			if atomic.LoadInt32(&s.paused) == 0 {
				sendFrame()
			}
		case <-s.visRequestChan:
			// Always send a frame on request. This is for paging when paused,
			// or for the initial frame on pause.
			sendFrame()
		}
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
			var stepsPerSecond int64 = 0
//			var stepsPerSecond = 1000000 * totalSteps / atomic.LoadInt64(&s.timeElapsed)

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
