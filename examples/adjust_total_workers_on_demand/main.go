// On this example:
//  - 10 workers will be running forever
//  - 30 jobs will be enqueued to be processed by the workers
//  - workers will exit after processing first 30 jobs

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

	// start up the workers (10 workers)
	pool.StartWorkers()

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			pool.AddTask(i)
		}

		// kill all active workers after current enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// set the number of live workers to 20
	pool.SetTotalWorkers(20)

	// wait while at least one worker is alive
	pool.Wait()
}
