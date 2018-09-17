## Go :: Pool of workers
Simple Pool of concurrent workers that allows to dynamically update / pause / resume live workers.

## Installation
```bash
go get gopkg.in/enriquebris/goworkerpool.v0
```

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
		for i := 0; i < 30; i++ {
			pool.AddTask(i)
		}
	}()

	// wait while at least one worker is alive
	pool.Wait()
}

```

#### Set the worker's function

Each time a worker receives a job, it invokes this function passing the job data as the only parameter.

```go
pool.SetWorkerFunc(func(data interface{}) bool {
		// do the processing

		// let the pool knows that the worker was able to complete the task
		return true
	})
```

#### Enqueue a job
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

#### Add an extra worker on the fly
```go
pool.AddWorker()
```

#### Kill a worker on the fly

Kill a live worker once it is idle or it finishes its current job.

```go
pool.KillWorker()
```

#### Set the number of workers on the fly

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


## History

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