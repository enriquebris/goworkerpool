[![godoc reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/enriquebris/goworkerpool) ![version](https://img.shields.io/badge/version-v0.7.2-yellowgreen.svg?style=flat "goworkerpool v0.7.2") [![Go Report Card](https://goreportcard.com/badge/github.com/enriquebris/goworkerpool)](https://goreportcard.com/report/github.com/enriquebris/goworkerpool)

# goworkerpool - Pool of workers
Pool of concurrent workers with the ability to increment / decrement / pause / resume workers on demand.

## Features

- [Change the number of workers on demand](#change-the-number-of-workers-on-demand)
- [Pause](#pause-all-workers) / [resume](#resume-all-workers) all workers
- Multiple ways to wait until jobs are done:
    - Wait until currently enqueued jobs get processed
    - Wait until n jobs get successfully processed
    - Wait until all workers are down

## Prerequisites

Golang version >= 1.9

## Installation
Execute:
```bash
go get -tags v0 github.com/enriquebris/goworkerpool

```

## Documentation
Visit [goworkerpool at godoc.org](https://godoc.org/github.com/enriquebris/goworkerpool)

## Examples

See code examples at [examples](examples/) folder.

## Get started

#### Simple usage

```go
package main

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
	// do not log messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberJobsInChannel, verbose)

	// add the worker function handler
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
		for i := 0; i < 30; i++ {
			pool.AddTask(i)
		}
		
		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}

```

### How To

#### Set the worker's function handler

Each time a worker receives a job, it invokes this function passing the job data as the only parameter.

```go
pool.SetWorkerFunc(func(data interface{}) bool {
		// do the processing

		// let the pool knows that the worker was able to complete the task
		return true
	})
```

#### Enqueue a job
*AddTask* will enqueue jobs in a FIFO way. Workers (if alive) will be listening to and picking up jobs from this queue.
If no workers are alive nor idle, the job will stay in the queue until any worker will be ready to pick it up and start processing it.

This queue has a limit (it was set up at pool initialization).

```go
pool.AddTask(data)
```

#### Pass multiple data to be processed by a worker

Let's suppose you have the following struct:

```go
type JobData struct {
	Filename string
	Path     string
	Size     uint64
}
```

then you can enqueue it as a job (to be processed by a worker):

```go
pool.AddTask(JobData{
	Filename: "file.txt",
	Path:     "/tmp/myfiles/",
	Size:     1500,
})
```

Keep in mind that the worker's function needs to cast the parameter as a JobData (that is on your side).

#### Add an extra worker on demand
```go
pool.AddWorker()
```

#### Kill a worker on demand

Kill a live worker once it is idle or it finishes its current job.

```go
pool.KillWorker()
```

#### <a name="change-the-number-of-workers-on-demand"></a>Change the number of workers on demand

Adjust the total of live workers by the given number.

```go
pool.SetTotalWorkers(n)
```

#### Kill all workers

Kill all live workers once they are idle or they finish processing their current jobs.

```go
pool.KillWorker()
```

#### Kill a worker after current enqueued jobs get processed

```go
pool.LateKillWorker()
```

#### Wait while at least one worker is alive

```go
pool.Wait()
```

#### Wait while n workers successfully finish their jobs

The worker function returns true or false. True means that the job was successfully finished. False means the opposite.

```go
pool.WaitUntilNSuccesses(n)
```

#### Pause all workers

All workers will be paused immediately after they finish processing their current jobs.
No new jobs will be taken from the queue until [ResumeAllWorkers()](#resume-all-workers) be invoked.

```go
pool.PauseAllWorkers()
```

#### Resume all workers

All workers will be resumed immediately.

```go
pool.ResumeAllWorkers()
```

## History

### v0.7.3

 - SetTotalWorkers() returns error in case it is invoked before StartWorkers()

### v0.7.2

 - Fixed bug that prevents to start/add new workers after a Wait() function finishes.

### v0.7.1

 - LateKillAllWorkers() will kill all alive workers (not only the number of workers that were alive when the function was invoked)

### v0.7

 - Repository name modified to "goworkerpool"

### v0.6

 - Pause / Resume all workers:
   - PauseAllWorkers() 
   - ResumeAllWorkers()
 - Workers will listen to higher priority channels first
 - Workers will listen to broad messages (kill all workers, ...) before get signals from any other channel:
   - KillAllWorkers()
   - KillAllWorkersAndWait()
 - Added function to kill all workers (send a broad message to all workers) and wait until it happens:
   - pool.KillAllWorkersAndWait()
 - Added code examples
   
   
### v0.5

 - Make Wait() listen to a channel (instead of use an endless for loop)
 
### v0.4

 - Sync actions over workers. A FIFO queue was created for the following actions:
   - Add new worker
   - Kill worker(s)
   - Late kill worker(s)
   - Set total workers 
   
### v0.3

 - Added function to adjust number of live workers:
   - pool.SetTotalWorkers(n)
 - Added function to kill all live workers after current jobs get processed:
   - pool.LateKillAllWorkers()
   
### v0.2

 - readme.md
 - godoc
 - code comments

### v0.1

 First stable BETA version.