package main

import (
	"fmt"
	"sync"
	"time"
)

const (
	numWorkers = 1000
	numJobs    = 100_000_000
	bufferSize = 1000
)

func main() {
	fmt.Printf("Testing with %d workers and %d jobs\n", numWorkers, numJobs)

	mutexRPS := testMutex()
	fmt.Printf("Mutex RPS: %.2f\n", mutexRPS)

	channelRPS := testChannel()
	fmt.Printf("Channel RPS: %.2f\n", channelRPS)

	fmt.Printf("Channel is %.2f%% faster than Mutex\n", (channelRPS-mutexRPS)/mutexRPS*100)
}

func testMutex() float64 {
	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numJobs/numWorkers; j++ {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return float64(numJobs) / elapsed.Seconds()
}

func testChannel() float64 {
	jobs := make(chan struct{}, bufferSize)
	results := make(chan struct{}, bufferSize)
	var wg sync.WaitGroup

	start := time.Now()

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				results <- struct{}{}
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < numJobs; i++ {
			jobs <- struct{}{}
		}
		close(jobs)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Count results
	count := 0
	for range results {
		count++
	}

	elapsed := time.Since(start)

	return float64(count) / elapsed.Seconds()
}
