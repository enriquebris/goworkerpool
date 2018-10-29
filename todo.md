# TODO

- [ ] Add categories to jobs.
- [x] Add a AddTaskCallback function to enqueue a callback as a job.
- [ ] [Catch concurrent panics caused by the worker handler function](#catch-concurrent-panics-caused-by-the-worker-handler-function).
- [ ] Provide better examples.


#### Catch concurrent panics caused by the worker handler function
There are certain panics caused by concurrent operations in the workers handler functions that bubble up and makes the app crash.

The following example is caused by a concurrent map writing:

```
fatal error: concurrent map writes

goroutine 20 [running]:
runtime.throw(0x10ca2f5, 0x15)
        /usr/local/Cellar/go/1.10.3/libexec/src/runtime/panic.go:616 +0x81 fp=0xc42003fdc8 sp=0xc42003fda8 pc=0x1026351
runtime.mapassign_faststr(0x10ad9e0, 0xc4200881b0, 0x10c8163, 0x7, 0x10a6000)
        /usr/local/Cellar/go/1.10.3/libexec/src/runtime/hashmap_fast.go:703 +0x3e9 fp=0xc42003fe38 sp=0xc42003fdc8 pc=0x1009ee9
main.main.func1(0x10a5960, 0xc420016088, 0xc42006a000)
        /Users/enrique.bris/golang/go/src/github.com/enriquebris/goworkerpool/examples/pool17.go:34 +0x7a fp=0xc42003fe78 sp=0xc42003fe38 pc=0x109575a
github.com/enriquebris/goworkerpool.(*Pool).workerFunc(0xc420098000, 0x3)
        /Users/enrique.bris/golang/go/src/github.com/enriquebris/goworkerpool/pool.go:615 +0x5aa fp=0xc42003ffb0 sp=0xc42003fe78 pc=0x10948da
github.com/enriquebris/goworkerpool.(*Pool).workerListener.func1(0xc420098000, 0x3)
        /Users/enrique.bris/golang/go/src/github.com/enriquebris/goworkerpool/pool.go:217 +0x51 fp=0xc42003ffd0 sp=0xc42003ffb0 pc=0x1095131
runtime.goexit()
        /usr/local/Cellar/go/1.10.3/libexec/src/runtime/asm_amd64.s:2361 +0x1 fp=0xc42003ffd8 sp=0xc42003ffd0 pc=0x104e031
created by github.com/enriquebris/goworkerpool.(*Pool).workerListener
        /Users/enrique.bris/golang/go/src/github.com/enriquebris/goworkerpool/pool.go:211 +0x7e

...
```

and the code that causes the panic:

```go
package main

import (
	"log"

	"github.com/enriquebris/goworkerpool"
)

func main() {
	// total workers
	totalWorkers := 10
	// max number of pending jobs
	maxNumberPendingJobs := 35
	// do not log messages about the pool processing
	verbose := false

	pool := goworkerpool.NewPool(totalWorkers, maxNumberPendingJobs, verbose)

	mp := make(map[string]int)

	// add the worker handler function
	pool.SetWorkerFunc(func(data interface{}) bool {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC ::::: '%v'\n", r)
			}
		}()

		// this will cause a panic when two workers attempt to write mp["Enrique"] at the same moment
		mp["Enrique"] = 50

		// let the pool knows that the worker was able to complete the task
		return true
	})

	// start up the workers
	pool.StartWorkers()

	// enqueue jobs in a separate goroutine
	go func() {
		for i := 0; i < 100; i++ {
			pool.AddTask(i)
		}

		// kill all workers after the currently enqueued jobs get processed
		pool.LateKillAllWorkers()
	}()

	// wait while at least one worker is alive
	pool.Wait()
}

```

Ok, this particular example could be solved by replacing the [map](https://blog.golang.org/go-maps-in-action) by [sync.map](https://golang.org/pkg/sync/#Map) (or a similar alternative), but the goal here is that goworkerpool catches the panic.