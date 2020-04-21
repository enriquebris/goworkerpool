// On this example:
//  - 10 workers will be started up
//  - the execution will wait until all 10 workers are alive
//  - 30 jobs will be enqueued to be processed by the workers
//  - all workers will be killed after the 30 enqueued jobs get processed
//	- a notification will be sent once a worker is killed (10 notifications will be received)

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

	// set the channel to receive notifications every time a worker is killed
	killedWorkerNotificationChannel := make(chan int, totalWorkers)
	pool.SetKilledWorkerChan(killedWorkerNotificationChannel)

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			pool.AddTask(i)
		}

		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// Instead of use pool.Wait() to wait until all workers are down, the following loop will be listening to the signals
	// sent once a worker is killed. The loop will exit after all initial workers were killed.
	totalKilledWorkers := 0
	// wait until all initial workers are alive
	for notification := range killedWorkerNotificationChannel {
		totalKilledWorkers = totalKilledWorkers + notification
		fmt.Printf("total killed workers: %v\n", totalKilledWorkers)

		if totalKilledWorkers == int(totalWorkers) {
			// break the loop once all initial workers are already up
			break
		}
	}

	fmt.Printf("All %v workers are down\n", totalWorkers)
}
