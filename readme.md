[![godoc reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/enriquebris/goworkerpool) ![version](https://img.shields.io/badge/version-v0.7.2-yellowgreen.svg?style=flat "goworkerpool v0.7.2")  [![Go Report Card](https://goreportcard.com/badge/github.com/enriquebris/goworkerpool)](https://goreportcard.com/report/github.com/enriquebris/goworkerpool)  [![Build Status](https://travis-ci.org/enriquebris/goworkerpool.svg?branch=master)](https://travis-ci.org/enriquebris/goworkerpool) 

# goworkerpool - Pool of workers
Pool of concurrent workers with the ability to increment / decrement / pause / resume workers on demand.

## Features

- [Enqueue jobs on demand](#enqueue-a-job)
- Multiple ways to wait / block
    - [Wait until at least a worker is alive](#wait-until-at-least-a-worker-is-alive)
    - [Wait until n jobs get successfully processed](#wait-until-n-jobs-get-successfully-processed)
- [Add](#add-workers-on-demand) / [kill](#kill-workers-on-demand) workers on demand
- Multiple ways to kill workers:
    - [On demand](#kill-workers-on-demand)
    - [After currently enqueued jobs get processed](#kill-workers-after-currently-enqueued-jobs-get-processed)
- [Change the number of workers on demand](#change-the-number-of-workers-on-demand)
- [Pause](#pause-all-workers) / [resume](#resume-all-workers) all workers


## Prerequisites

Golang version >= 1.9

## Installation
Execute:
```bash
go get -tags v0 github.com/enriquebris/goworkerpool

```

## Documentation
Visit [goworkerpool at godoc.org](https://godoc.org/github.com/enriquebris/goworkerpool)

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
	// max number of pending jobs in queue
	maxNumberPendingJobsInQueue := 15
	// do not log messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberPendingJobsInQueue, verbose)

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

## Examples

See code examples at [examples](examples/) folder.


## How To

### Set the worker's handler function

```go
pool.SetWorkerFunc(fn)
```

This is the way to set the function that will be invoked from a worker each time it pulls a job from the queue.
Internally, the worker will pass the job as a parameter to this function.

The function signature ([PoolFunc](https://godoc.org/github.com/enriquebris/goworkerpool#PoolFunc)):

```go
func handler(interface{}) bool {
	
}
```

The handler function should return true to let know that the job was successfully processed, or false in other case.

```go
pool.SetWorkerFunc(func(data interface{}) bool {
		// do the processing

		// let the pool knows that the worker was able to complete the task
		return true
	})
```

### Enqueue a job

[AddTask](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddTask) will enqueue a job into a FIFO queue (a channel).

```go
pool.AddTask(data)
```

The parameter for the job's data accepts any kind of value (interface{}).

Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
the job will stay in the queue until any worker will be ready to pick it up and start processing it.

The queue in which this function enqueues the jobs has a limit (it was set up at [pool initialization](https://godoc.org/github.com/enriquebris/goworkerpool#NewPool)). It means that AddTask will wait
for a free queue slot to enqueue a new job in case the queue is at full capacity.

AddTask will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
a certain amount of time when [WaitUntilNSuccesses](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.WaitUntilNSuccesses) meet the stop condition.

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

Keep in mind that the worker's handler function needs to cast the parameter as a JobData (that is on your side).

### Wait until at least a worker is alive

[Wait](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.Wait) blocks until at least one worker is alive.

```go
pool.Wait()
```

### Wait until n jobs get successfully processed

[WaitUntilNSuccesses](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.WaitUntilNSuccesses) blocks until n jobs get successfully processed.

```go
pool.WaitUntilNSuccesses(n)
```

### Add workers on demand
#### Add a worker on demand

[AddWorker](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddWorker) adds a new worker to the pool.

```go
pool.AddWorker()
```

#### Add n workers on demand

[AddWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddWorkers) adds n new workers to the pool.

```go
pool.AddWorkers(n)
```

### Multiple ways to kill workers

#### Kill workers on demand
##### Kill a worker

[KillWorker](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillWorker) kills a live worker once it is idle or after it finishes with its current job.

```go
pool.KillWorker()
```

##### Kill n workers

[KillWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillWorkers) kills all live workers. For those currently processing jobs, it will wait until the work is done.

```go
pool.KillWorkers(n)
```

##### Kill all workers

[KillAllWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillAllWorkers) kills all live workers once they are idle or after they finish processing their current jobs.

```go
pool.KillAllWorkers()
```

##### Kill all workers and wait

[KillAllWorkersAndWait](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillAllWorkersAndWait) triggers an action to kill all live workers and blocks until the action is done (meaning that all live workers are down).

```go
pool.KillAllWorkersAndWait()
```

#### Kill workers after currently enqueued jobs get processed
##### Kill a worker after currently enqueued jobs get processed

[LateKillWorker](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillWorker) kills a worker after currently enqueued jobs get processed.
If the worker is processing a job, it will be killed after the job gets processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillWorker()
```

##### Kill n workers after currently enqueued jobs get processed

[LateKillWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillWorkers) kills n workers after currently enqueued jobs get processed.
If the workers are processing jobs, they will be killed after the jobs get processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillWorkers(n)
```

##### Kill all workers after currently enqueued jobs get processed

[LateKillAllWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillAllWorkers) kills all workers after currently enqueued jobs get processed.
For those workers currently processing jobs, they will be killed after the jobs get processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillAllWorkers()
```

### <a name="change-the-number-of-workers-on-demand"></a>Change the number of workers on demand

[SetTotalWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.SetTotalWorkers) adjusts the number of live workers.

```go
pool.SetTotalWorkers(n)
```

In case it needs to kill some workers (in order to adjust the total based on the given parameter), it will wait until their current jobs get processed (if they are processing jobs).

It returns an error in the following scenarios:
 - The workers were not started yet by StartWorkers.
 - There is a "in course" KillAllWorkers operation.

### Pause all workers

[PauseAllWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.PauseAllWorkers) immediately pauses all workers (after they finish processing their current jobs).
No new jobs will be pulled from the queue until [ResumeAllWorkers()](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.ResumeAllWorkers) be invoked.

```go
pool.PauseAllWorkers()
```

### Resume all workers

[ResumeAllWorkers()](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.ResumeAllWorkers) resumes all workers.

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