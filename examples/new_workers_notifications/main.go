// On this example:
//  - 10 workers will be started up
//  - the execution will wait until all 10 workers are alive
//  - 30 jobs will be enqueued to be processed by the workers
//  - all workers will be killed after the 30 enqueued jobs get processed

package main

import (
	"log"
	"time"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	// total workers
	totalInitialWorkers := 10
	// max number of pending jobs
	maxNumberPendingJobs := 15
	// do not log messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalInitialWorkers, maxNumberPendingJobs, verbose)

	// add the worker handler function
	pool.SetWorkerFunc(func(data interface{}) bool {
		log.Printf("processing %v\n", data)
		// add a 1 second delay (to makes it look as it were processing the job)
		time.Sleep(time.Second)
		log.Printf("processing finished for: %v\n", data)

		// let the pool knows that the worker was able to complete the task
		return true
	})

	// set the channel to receive notifications every time a new worker is started up
	newWorkerNotificationChannel := make(chan int)
	pool.SetNewWorkerChan(newWorkerNotificationChannel)

	// Note that the following lines (44 to 55) could be replaced by pool.StartWorkersAndWait() to achieve the
	// same goal: wait until all workers are up. This code is intended as an example.

	// start up the workers
	pool.StartWorkers()

	totalWorkersUp := 0
	// wait until all initial workers are alive
	for notification := range newWorkerNotificationChannel {
		totalWorkersUp = totalWorkersUp + notification
		if totalWorkersUp == totalInitialWorkers {
			// break the loop once all initial workers are already up
			break
		}
	}

	log.Printf("%v workers are up\n", totalWorkersUp)

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			pool.AddTask(i)
		}

		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}
