// On this example:
//  - 10 workers will be running forever
//  - 30 jobs will be enqueued to be processed by the workers
//  - workers will exit after processing first 30 jobs

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
		TotalInitialWorkers:  totalWorkers,
		MaxOperationsInQueue: maxNumberPendingJobs,
		MaxWorkers:           20,
		LogVerbose:           verbose,
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

	// start up the workers and wait until them are up
	pool.WaitUntilInitialWorkersAreUp()

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
