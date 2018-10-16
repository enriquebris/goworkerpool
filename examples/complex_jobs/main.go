// On this example:
//  - 10 workers will be running
//  - 30 jobs (having multiple data ==> a custom struct) will be enqueued
//  - workers will exit after processing first 30 jobs

package main

import (
	"fmt"
	"time"

	"github.com/enriquebris/goworkerpool"
)

// JobData holds the job's data
type JobData struct {
	Name            string
	PictureFilePath string
}

func main() {
	// total workers
	totalWorkers := 10
	// max number of tasks waiting in the channel
	maxNumberJobsInChannel := 15
	// do not show messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberJobsInChannel, verbose)

	// add the worker function
	pool.SetWorkerFunc(worker)

	// start up the workers
	pool.StartWorkers()

	// add tasks in a separate goroutine
	go func() {
		for i := 0; i < 30; i++ {
			// the jobs contain a custom struct: JobData
			pool.AddTask(JobData{
				Name:            fmt.Sprintf("John-%v", i+1),
				PictureFilePath: fmt.Sprintf("/pictures/john-%v", i+1),
			})
		}

		// kill all workers after the current enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}

// worker is the function each pool's worker will invoke to process jobs
func worker(data interface{}) bool {
	// cast the interface{} back to our custom JobData
	if job, ok := data.(JobData); ok {
		fmt.Printf("processing %v\npicture: %v\n\n", job.Name, job.PictureFilePath)
		// process the job here
		time.Sleep(1 * time.Second)
		fmt.Printf("\n'%v' was processed\n", job.Name)

		// let the pool know that this job was successfully processed
		return true
	}

	// the job was not successfully processed
	return false
}
