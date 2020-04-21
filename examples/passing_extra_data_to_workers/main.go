// On this example:
//  - 10 workers will be running
//  - the worker's function will be a struct function having access to all other members of the struct
//  - 30 jobs will be enqueued

package main

import (
	"fmt"
	"time"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	var (
		// total workers
		totalWorkers uint = 10
		// max number of pending jobs
		maxNumberPendingJobs uint = 150
		// do not show messages about the pool processing
		verbose = false
	)

	pool, err := goworkerpool.NewPoolWithOptions(goworkerpool.PoolOptions{
		TotalInitialWorkers:          totalWorkers,
		MaxOperationsInQueue:         maxNumberPendingJobs,
		MaxWorkers:                   20,
		WaitUntilInitialWorkersAreUp: true,
		LogVerbose:                   verbose,
	})

	if err != nil {
		fmt.Println(err)
		return
	}

	// CustomWorker
	customWorker := NewCustomWorker()

	// add the worker handler function
	pool.SetWorkerFunc(customWorker.Worker)

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			// enqueue a job
			pool.AddTask(i)
		}

		// kill all workers after the current enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}

type CustomWorker struct {
	creationDate time.Time
}

// NewCustomWorker returns a fresh *CustomWorker
func NewCustomWorker() *CustomWorker {
	return &CustomWorker{
		creationDate: time.Now(),
	}
}

// worker is the function each pool's worker will invoke to process jobs
func (st *CustomWorker) Worker(data interface{}) bool {
	// cast the interface{} back to int
	if job, ok := data.(int); ok {
		fmt.Printf("processing job #%v\n", job)
		// process the job here
		time.Sleep(1 * time.Second)
		fmt.Printf("\njob #%v was processed\n", job)

		// The worker's function can access any member of *CustomWorker, for example: st.creationDate

		// let the pool know that this job was successfully processed
		return true
	}

	// the job was not successfully processed
	return false
}
