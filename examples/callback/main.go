// On this example:
//  - 30 jobs will be enqueued to be processed by the workers
//  - 10 workers will be running until the 30 enqueued jobs get processed
//	- 1 of the jobs will be enqueued plus a callback, which will be invoked just after the job get processed
//  - there is a callback to enqueue without job's data

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
	// do not log messages about the pool processing
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

	// start up the workers and wait until them are up
	pool.StartWorkersAndWait()

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			if i == 15 {
				// enqueue a job + callback
				// the job will be processed first and later the callback will be invoked
				pool.AddTaskCallback(i, func(data interface{}) {
					log.Printf("Callback for: '%v' *******************************", data)
				})
				continue
			}

			if i == 20 {
				// enqueue a callback
				// there is no job to process, so the only thing the worker will do is to invoke the callback function
				pool.AddCallback(func(data interface{}) {
					log.Println("simple callback function")
				})
				continue
			}

			pool.AddTask(i)
		}

		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}
