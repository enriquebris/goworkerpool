// On this example:
//  - 10 workers will be running
//  - 100 jobs will be enqueued
//  - all workers will exit after 30 jobs are successfully processed

package main

import (
	"fmt"
	"log"
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

	// add the worker handler function
	pool.SetWorkerFunc(func(data interface{}) bool {
		log.Printf("processing %v\n", data)
		// add a 1 second delay (to makes it look as it were processing the job)
		time.Sleep(time.Second)
		log.Printf("processing finished for: %v\n", data)

		// let the pool knows that the worker was able to complete the task
		return true
	})

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 100; i++ {
			pool.AddTask(i)
		}
	}()

	// wait until at least 30 jobs get successfully processed
	pool.WaitUntilNSuccesses(30)
}
