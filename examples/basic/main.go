// On this example:
//  - 10 workers will be started up
//  - 30 jobs will be enqueued to be processed by the workers
//	- all workers will be killed after the 30 enqueued jobs get processed

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	var (
		maxOperationsInQueue uint = 50
	)
	pool, err := goworkerpool.NewPoolWithOptions(goworkerpool.PoolOptions{
		TotalInitialWorkers:          10,
		MaxWorkers:                   20,
		MaxOperationsInQueue:         maxOperationsInQueue,
		WaitUntilInitialWorkersAreUp: true,
		LogVerbose:                   true,
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
		for i := 0; i < int(maxOperationsInQueue); i++ {
			pool.AddTask(i)
		}

		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}
