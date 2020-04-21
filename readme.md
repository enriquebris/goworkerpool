[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/mod/github.com/enriquebris/goworkerpool) [![godoc reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/enriquebris/goworkerpool) ![version](https://img.shields.io/badge/version-v0.10.0-yellowgreen.svg?style=flat "goworkerpool v0.10.0")  [![Go Report Card](https://goreportcard.com/badge/github.com/enriquebris/goworkerpool)](https://goreportcard.com/report/github.com/enriquebris/goworkerpool)  [![Build Status](https://travis-ci.org/enriquebris/goworkerpool.svg?branch=master)](https://travis-ci.org/enriquebris/goworkerpool) [![codecov](https://codecov.io/gh/enriquebris/goworkerpool/branch/master/graph/badge.svg)](https://codecov.io/gh/enriquebris/goworkerpool) 

# goworkerpool - Pool of workers
Pool of concurrent workers with the ability of increment / decrement / pause / resume workers on demand.

## Features

- [Enqueue jobs on demand](#enqueue-jobs-on-demand)
- Multiple ways to wait / block
    - [Wait until at least a worker is alive](#wait-until-at-least-a-worker-is-alive)
    - [Wait until n jobs get successfully processed](#wait-until-n-jobs-get-successfully-processed)
- [Add](#add-workers-on-demand) / [kill](#kill-workers-on-demand) workers on demand
- Be notified once a worker is [started up](#receive-a-notification-every-time-a-new-worker-is-started-up) / [killed](#receive-a-notification-every-time-a-worker-is-killed)
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

## TODO
Visit the [TODO](./todo.md) page.

## Get started

#### Simple usage

```go
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

### Start up the workers

#### Start up the workers asynchronously
[StartWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.StartWorkers) spins up the workers. The amount of workers to be started up is defined at the Pool instantiation.

```go
pool.StartWorkers()
```

This is an asynchronous operation, but there is a way to be be notified each time a new worker is started up: through a channel. See [SetNewWorkerChan(chan)](#receive-a-notification-every-time-a-new-worker-is-started-up).

#### Start up the workers synchronously

[StartWorkersAndWait](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.StartWorkersAndWait) spins up the workers and wait until all of them are 100% up. The amount of workers to be started up is defined at the Pool instantiation.

```go
pool.StartWorkersAndWait()
```

Although this is an synchronous operation, there is a way to be be notified each time a new worker is started up: through a channel. See [SetNewWorkerChan(chan)](#receive-a-notification-every-time-a-new-worker-is-started-up). Keep in mind that the channel listener should be running on a different goroutine.

### Receive a notification every time a new worker is started up

[SetNewWorkerChan(chan)](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.SetNewWorkerChan) sets a channel to receive notifications every time a new worker is started up.

```go
pool.SetNewWorkerChan(ch chan<- int)
```

This is optional, no channel is needed to start up new workers. Basically is just a way to give feedback for the worker's start up operation.

### Enqueue jobs on demand
#### Enqueue a simple job

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
a certain amount of time when [WaitUntilNSuccesses](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.WaitUntilNSuccesses) meets the stop condition.

#### Enqueue a simple job plus callback function

[AddTaskCallback](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddTaskCallback) will enqueue a job plus a callback function into a FIFO queue (a channel).

```go
pool.AddTaskCallback(data, callback)
```

The parameter for the job's data accepts any type (interface{}).

Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
the job will stay in the queue until any worker will be ready to pick it up and start processing it.

The worker who picks up this job + callback will process the job first and later will invoke the callback function, passing the job's data as a parameter.

The queue in which this function enqueues the jobs has a limit (it was set up at pool initialization). It means that AddTaskCallback will wait
for a free queue slot to enqueue a new job in case the queue is at full capacity.

AddTaskCallback will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
a certain amount of time when [WaitUntilNSuccesses](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.WaitUntilNSuccesses) meets the stop condition.

#### Enqueue a callback function

[AddCallback](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddCallback) will enqueue a callback function into a FIFO queue (a channel).

```go
pool.AddCallback(callback)
```

Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
the job will stay in the queue until any worker will be ready to pick it up and start processing it.

The worker who picks up this task will only invoke the callback function, passing nil as a parameter.

The queue in which this function enqueues the jobs has a limit (it was set up at pool initialization). It means that AddCallback will wait
for a free queue slot to enqueue a new job in case the queue is at full capacity.

AddCallback will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
a certain amount of time when [WaitUntilNSuccesses](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.WaitUntilNSuccesses) meets the stop condition.

#### Enqueue a job with category and callback

[AddComplexTask](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddComplexTask) will enqueue a job into a FIFO queue (a channel).

```go
pool.AddComplexTask(data, category, callback)
```

This function extends the scope of [AddTask](#enqueue-a-simple-job) adding category and callback.

The job will be grouped based on the given category (for stats purposes).

The callback function (if any) will be invoked just after the job gets processed. 

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

This is an asynchronous operation, but there is a [way to be be notified each time a new worker is started up: through a channel](#receive-a-notification-every-time-a-new-worker-is-started-up).

#### Add n workers on demand

[AddWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.AddWorkers) adds n new workers to the pool.

```go
pool.AddWorkers(n)
```

This is an asynchronous operation, but there is a [way to be be notified each time a new worker is started up: through a channel](#receive-a-notification-every-time-a-new-worker-is-started-up).

### Multiple ways to kill workers

#### Kill workers on demand
##### Kill a worker

[KillWorker](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillWorker) kills a live worker once it is idle or after it finishes with its current job.

```go
pool.KillWorker()
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

##### Kill n workers

[KillWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillWorkers) kills all live workers. For those currently processing jobs, it will wait until the work is done.

```go
pool.KillWorkers(n)
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

##### Kill all workers

[KillAllWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillAllWorkers) kills all live workers once they are idle or after they finish processing their current jobs.

```go
pool.KillAllWorkers()
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

##### Kill all workers and wait

[KillAllWorkersAndWait](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.KillAllWorkersAndWait) triggers an action to kill all live workers and blocks until the action is done (meaning that all live workers are down).

```go
pool.KillAllWorkersAndWait()
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

#### Kill workers after currently enqueued jobs get processed
##### Kill a worker after currently enqueued jobs get processed

[LateKillWorker](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillWorker) kills a worker after currently enqueued jobs get processed.
If the worker is processing a job, it will be killed after the job gets processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillWorker()
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

##### Kill n workers after currently enqueued jobs get processed

[LateKillWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillWorkers) kills n workers after currently enqueued jobs get processed.
If the workers are processing jobs, they will be killed after the jobs get processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillWorkers(n)
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

##### Kill all workers after currently enqueued jobs get processed

[LateKillAllWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.LateKillAllWorkers) kills all workers after currently enqueued jobs get processed.
For those workers currently processing jobs, they will be killed after the jobs get processed.

By "*currently enqueued jobs*" I mean: the jobs enqueued at the moment this function was invoked.

```go
pool.LateKillAllWorkers()
```

This is an asynchronous operation, but there is a way to be be notified each time a worker is killed: through a channel. See [SetKilledWorkerChan(chan)](#receive-a-notification-every-time-a-worker-is-killed).

### Receive a notification every time a worker is killed

[SetKilledWorkerChan(chan)](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.SetKilledWorkerChan) sets a channel to receive notifications every time a worker is killed.

```go
pool.SetKilledWorkerChan(ch chan int)
```

This is 100% optional.


### <a name="change-the-number-of-workers-on-demand"></a>Update the amount of workers on demand

[SetTotalWorkers](https://godoc.org/github.com/enriquebris/goworkerpool#Pool.SetTotalWorkers) adjusts the number of live workers.

```go
pool.SetTotalWorkers(n)
```

In case it needs to kill some workers (in order to adjust the total based on the given parameter), it will wait until their current jobs get processed (if they are processing jobs).

This is an asynchronous operation, but there is a [way to be be notified each time a new worker is started up: through a channel](#receive-a-notification-every-time-a-new-worker-is-started-up).

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

### v0.10.0

- Initial workers automatically start running on pool initialization
    - deprecated StartWorkers()
    
- Each new added worker is being automatically started

- **SafeWaitUntilNSuccesses**: it waits until *n* tasks were successfully processed, but if any extra task is already "in progress", this function will wait until it is done. An extra enqueued task could started processing just before the *nth* expected task was finished.

- **GetTotalWorkersInProgress**: returns total workers in progress.

- **KillAllWorkers** returns error
- **KillAllWorkersAndWait** returns error

- **SetTotalWorkers** It won't return error because of the workers were not yet started, workers are now started once they are created.

- **WaitUntilInitialWorkersAreUp**: it waits until all initial workers are up and running.

- **StartWorkers** is deprecated. It only returns nil.
- **StartWorkersAndWait** is deprecated. It returns WaitUntilInitialWorkersAreUp()

### v0.9.1

- Examples: Replaced pool.StartWorkers() by pool.StartWorkersAndWait()

### v0.9.0

- Added a way to know that new workers were started (using an optional channel)
- Added a way to know if a worker was killed (using an optional channel)
- StartWorkersAndWait() to start workers (for first time) and wait until all of them are alive

### v0.8.0

 - Enqueue jobs plus callback functions
 - Enqueue callback functions without jobs' data

### v0.7.4

 - Fixed bug that caused randomly worker initialization error

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
