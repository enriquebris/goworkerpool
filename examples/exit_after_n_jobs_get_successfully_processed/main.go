// On this example:
//  - 10 workers will be running
//  - 100 jobs will be enqueued
//  - all workers will exit after 30 jobs are successfully processed

package main

import (
	"log"
	"time"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	// total workers
	totalWorkers := 10
	// max number of pending jobs
	maxNumberPendingJobs := 15
	// do not show messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberPendingJobs, verbose)

	// add the worker handler function
	pool.SetWorkerFunc(func(data interface{}) bool {
		log.Printf("processing %v\n", data)
		// add a 1 second delay (to makes it look as it were processing the job)
		time.Sleep(time.Second)
		log.Printf("processing finished for: %v\n", data)

		// let the pool knows that the worker was able to complete the task
		return true
	})

	// start up the workers
	pool.StartWorkers()

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 100; i++ {
			pool.AddTask(i)
		}
	}()

	// wait until 30 jobs get successfully processed
	pool.WaitUntilNSuccesses(30)
}
