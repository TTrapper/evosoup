package main

import (
	"context"
	"evolution/vm"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// --- Simulation Constants ---
const (
	SoupSize              = 1024 * 1024
	InitialNumIPs         = 4096 * 128
	GenerationTimeSeconds = 1
	MinStepsPerGen        = 1 // Minimum steps to be considered "alive"
)

// runIP is the function that executes in each IP's goroutine.
func runIP(p *vm.IP, ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done(): // The generation time is up
			return
		default:
			p.Step()
		}
	}
}

func main() {
	fmt.Println("--- EvoSoup: A Go-based Artificial Life Simulation ---")

	// --- Initialization ---
	soup := make([]int32, SoupSize)
	for i := range soup {
		soup[i] = int32(rand.Intn(9))
	}

	// The master list of IPs is now a sync.Map for concurrent-safe access.
	var population sync.Map
	var nextIPID int32 = 0

	// Initial population.
	for i := 0; i < InitialNumIPs; i++ {
		startPtr := rand.Int31n(SoupSize)
		newID := atomic.AddInt32(&nextIPID, 1)
		ip := vm.NewIP(int(newID), soup, startPtr)
		population.Store(ip.ID, ip)
	}

	initialCount := 0
	population.Range(func(_, _ interface{}) bool {
		initialCount++
		return true
	})
	fmt.Printf("Simulation started with %d IPs in a soup of %d instructions.\n", initialCount, SoupSize)

	// --- Main Generation Loop ---
	for gen := 1; ; gen++ {
		fmt.Printf("\n--- Generation %d ---\n", gen)

		// --- Per-Generation State ---
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), GenerationTimeSeconds*time.Second)

		// Start goroutines for the initial population of this generation.
		// We must iterate over the map and inject the required concurrency tools into each IP.
		population.Range(func(key, value interface{}) bool {
			ip := value.(*vm.IP)

			// Reset state and inject tools
			ip.Steps = 0
			ip.SpawnAttempt = 0
			for i := range ip.OpcodeCounts {
				ip.OpcodeCounts[i] = 0
			}
			ip.Population = &population
			ip.Wg = &wg
			ip.NextIPID = &nextIPID
			ip.Ctx = ctx
			ip.RunIP = runIP

			wg.Add(1)
			go runIP(ip, ctx, &wg)
			return true // continue iteration
		})

		wg.Wait() // Wait for all goroutines (including spawned children) to finish.
		cancel()  // Clean up context resources.

		fmt.Println("Generation finished. Evaluating fitness...")

		// --- Culling and Replication ---
		var totalSteps int64
		var successfulSpawns int64
		var opcodeCounts [vm.NumOpcodes]int64
		currentPopSize := 0

		newPopulation := sync.Map{}
		aliveCount := 0

		population.Range(func(key, value interface{}) bool {
			currentPopSize++
			ip := value.(*vm.IP)
			totalSteps += ip.Steps
			// In this model, every attempt is a success, so we count them directly.
			successfulSpawns += ip.SpawnAttempt
			for i, count := range ip.OpcodeCounts {
				opcodeCounts[i] += count
			}

			if ip.Steps > 0 {
				newPopulation.Store(key, value)
				aliveCount++
			}
			return true
		})

		fmt.Printf("Spawn Successful/Attempts: %d.\n", successfulSpawns)
		fmt.Printf("Total steps: %d. Population size: %d -> %d (alive).\n", totalSteps, currentPopSize, aliveCount)

		fmt.Println("Opcode Execution Counts:")
		for i, count := range opcodeCounts {
			fmt.Printf("  %s: %d\n", vm.OpcodeNames[i], count)
		}

		if aliveCount == 0 {
			fmt.Println("Extinction event! No IPs survived. Re-seeding population.")
			// Clear the map for a fresh start
			population = newPopulation
			for i := 0; i < InitialNumIPs; i++ {
				startPtr := rand.Int31n(SoupSize)
				newID := atomic.AddInt32(&nextIPID, 1)
				ip := vm.NewIP(int(newID), soup, startPtr)
				population.Store(ip.ID, ip)
			}
		} else {
			population = newPopulation
		}

	}
}
