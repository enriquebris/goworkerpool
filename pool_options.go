// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

// PoolOptions contains configuration data to build a Pool using NewPoolWithOptions
type PoolOptions struct {
	// amount of initial workers
	TotalInitialWorkers uint
	// maximum amount of workers
	MaxWorkers uint
	// maximum actions/tasks to be enqueued at the same time
	MaxOperationsInQueue uint
	// channel to send notifications once new workers are created
	NewWorkerChan chan int
	// wait (NewPoolWithOptions function) until all initial workers are up and running
	WaitUntilInitialWorkersAreUp bool
	// log's verbose
	LogVerbose bool
}
