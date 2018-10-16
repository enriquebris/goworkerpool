// On this example:
//  - 10 workers will be running
//  - 100 jobs will be enqueued
//  - all workers will exit after 30 jobs are successfully processed

package exit_after_n_jobs_get_successfully_processed

import (
	"log"
	"time"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	// total workers
	totalWorkers := 10
	// max number of tasks waiting in the channel
	maxNumberJobsInChannel := 15
	// do not show messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberJobsInChannel, verbose)

	// add the worker function
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

	// add tasks in a separate goroutine
	go func() {
		for i := 0; i < 100; i++ {
			pool.AddTask(i)
		}
	}()

	// wait while at least one worker is alive
	pool.WaitUntilNSuccesses(30)
}
