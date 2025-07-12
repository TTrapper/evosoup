package main

import (
	"context"
	"encoding/gob"
	"evolution/vm"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- Simulation Constants ---
const (
	SoupSize              = 1024 * 1024 * 16
	InitialNumIPs         = 4096 * 128
	GenerationTimeSeconds = 1
	MinStepsPerGen        = 1 // Minimum steps to be considered "alive"
	SnapshotInterval      = 10 // Save a snapshot every 100 generations
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

	// --- Initialization ---
	seed := time.Now().UnixNano()
	rand.Seed(seed)

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
	fmt.Printf("Simulation started with %d IPs in a soup of %d instructions. Seed: %d\n", initialCount, SoupSize, seed)

	// --- Main Generation Loop ---
	for gen := 1; ; gen++ {
		// --- Per-Generation State ---
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), GenerationTimeSeconds*time.Second)

		// Start goroutines for the initial population of this generation.
		population.Range(func(key, value interface{}) bool {
			ip := value.(*vm.IP)

			// Reset state and inject tools
			ip.Steps = 0
			ip.SpawnAttempt = 0
			ip.SpawnedGenotypes = ip.SpawnedGenotypes[:0] // Reset for new generation
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

		// --- Culling, Replication, and Analysis ---
		var totalSteps int64
		var successfulSpawns int64

		phenotypeCounts := make(map[string]int)
		genotypeCounts := make(map[uint64]int)

		newPopulation := sync.Map{}
		aliveCount := 0

		population.Range(func(key, value interface{}) bool {
			ip := value.(*vm.IP)
			totalSteps += ip.Steps
			successfulSpawns += ip.SpawnAttempt

			// Aggregate genotypes spawned by this IP
			for _, genotypeHash := range ip.SpawnedGenotypes {
				genotypeCounts[genotypeHash]++
			}

			if ip.Steps > MinStepsPerGen {
				newPopulation.Store(key, value)
				aliveCount++

				// Calculate phenotype for this surviving IP
				var keyBuilder strings.Builder
				for i, count := range ip.OpcodeCounts {
					normalized := int(math.Round((float64(count) / float64(ip.Steps)) * 100))
					keyBuilder.WriteString(fmt.Sprintf("%d", normalized))
					if i < len(ip.OpcodeCounts)-1 {
						keyBuilder.WriteString(",")
					}
				}
				phenotypeKey := keyBuilder.String()
				phenotypeCounts[phenotypeKey]++
			}
			return true
		})

		if aliveCount == 0 {
			fmt.Println("Extinction event! No IPs survived. Re-seeding population.")
			population = sync.Map{}
			for i := 0; i < InitialNumIPs; i++ {
				startPtr := rand.Int31n(SoupSize)
				newID := atomic.AddInt32(&nextIPID, 1)
				ip := vm.NewIP(int(newID), soup, startPtr)
				population.Store(ip.ID, ip)
			}
		} else {
			population = newPopulation
		}

		// --- Diversity Metrics Calculation ---
		// Phenotype Diversity
		numPhenotypes := len(phenotypeCounts)
		domPhenoCount := 0
		for _, count := range phenotypeCounts {
			if count > domPhenoCount {
				domPhenoCount = count
			}
		}
		domPhenoPct := 0.0
		if aliveCount > 0 {
			domPhenoPct = (float64(domPhenoCount) / float64(aliveCount)) * 100
		}

		// Genotype Diversity
		numGenotypes := len(genotypeCounts)
		domGenoCount := 0
		for _, count := range genotypeCounts {
			if count > domGenoCount {
				domGenoCount = count
			}
		}
		domGenoPct := 0.0
		if successfulSpawns > 0 {
			domGenoPct = (float64(domGenoCount) / float64(successfulSpawns)) * 100
		}

		// Soup Entropy
		soupCounts := make(map[int32]int)
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

		// --- Consolidated Logging ---
		fmt.Printf("Gen: %-6d | Pop: %-5d | Steps: %-10d | Spawns: %-5d",
			gen, aliveCount, totalSteps, successfulSpawns)
		fmt.Printf(" | Phenos: %-4d (Dom: %2.0f%%) | Genos: %-4d (Dom: %2.0f%%) | Entropy: %-4.2f",
			numPhenotypes, domPhenoPct, numGenotypes, domGenoPct, soupEntropy)

		// --- Snapshotting ---
		if gen%SnapshotInterval == 0 {
			var savableIPs []vm.SavableIP
			population.Range(func(key, value interface{}) bool {
				ip := value.(*vm.IP)
				savableIPs = append(savableIPs, ip.Savable())
				return true
			})

			snapshotState := SimulationState{
				Generation: gen,
				Soup:       soup,
				IPs:        savableIPs,
				NextIPID:   atomic.LoadInt32(&nextIPID),
				RandSeed:   seed,
			}

			snapshotFilename := "snapshot.gob"
			if err := saveSnapshot(snapshotState, snapshotFilename); err != nil {
				fmt.Printf(" (Error saving snapshot: %v)\n", err)
			} else {
				fmt.Printf(" (Snapshot saved to %s)\n", snapshotFilename)
			}
		} else {
			fmt.Println()
		}
	}
}
