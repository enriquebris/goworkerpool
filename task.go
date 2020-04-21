// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

const (
	taskRegular    = 10 // regular task
	taskLateAction = 50 // late action (enqueued at the same tasks queue)
)

// task represents a task / job
type task struct {
	// task's data
	Code int
	// task/job/data
	Data interface{}
	// function to process the data
	Func PoolFunc
	// callback function
	CallbackFunc PoolCallback
	// tells whether the task is valid. Invalid tasks should be skipped by the operationHandler
	Valid bool
}
