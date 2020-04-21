// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"fmt"
	"sync"
	"time"

	"github.com/enriquebris/goconcurrentcounter"
)

const (
	ErrorWorkerNilTask    = "worker.nil.task"
	ErrorWorkerNoTaskFunc = "worker.no.func.to.process.task"
	ListenInProgress      = "listen.in.progress"
	WorkerClosed          = "worker.closed"
)

type (
	// external action: func() (reEnqueueWorker bool)
	// reEnqueueWorker bool ==> let the pool knows whether the worker should be re-enqueued in the availableWorkers queue
	workerExternalActionFunc func() (reEnqueueWorker bool)
	// internal action: function() (bool), return keepWorking
	workerInternalActionFunc func() bool
	// function to be executed by the worker to provide feedback about the action/task processing
	workerFeedbackFunc func(wkrID int, reEnqueueWorker bool)
	// external function to be executed by the worker
	workerExternalGeneralFunc func()
	// external function to be called by the worker if the handler fails
	workerExternalFailFunc func(tsk *task, err error)
)

// workerFeedback holds the worker's feedback data
type workerFeedback struct {
	workerID        int
	reEnqueueWorker bool
}

// workerFunctionData holds the worker's function result
type workerFunctionData struct {
	result bool
	err    error
}

type worker struct {
	id          int
	tasksChan   chan *task
	actionsChan chan action
	// to let Listen knows that Close() was executed
	closeChan chan chan struct{}
	// to let know Listen is not longer listening
	closeListenChan chan struct{}
	// action:internal function
	internalActions map[string]workerInternalActionFunc
	// action: external function
	externalActions map[string]workerExternalActionFunc

	// external functions
	// external function to provide feedback about the action/task processing
	externalFeedbackFunc workerFeedbackFunc
	// external function to be executed just before start processing an action
	externalPreActionFunc workerExternalGeneralFunc
	// external function to be executed after an action gets processed
	externalPostActionFunc workerExternalGeneralFunc
	// external function to be executed just before start processing a task
	externalPreTaskFunc workerExternalGeneralFunc
	// external function to be executed after a task gets processed
	externalPostTaskFunc workerExternalGeneralFunc
	// external function to be called by the worker if the handler fails
	externalFailFunc workerExternalFailFunc

	// error channel to send errors
	errorChan chan *PoolError

	// metrics about the tasks' executions
	taskSuccesses         int
	externalTaskSuccesses goconcurrentcounter.Int
	taskFailures          int

	// general information map (actions in progress, etc)
	generalInfoMap sync.Map
}

// newWorker builds and returns a new worker
func newWorker(id int) *worker {
	ret := &worker{}
	ret.initialize(id)

	return ret
}

func (st *worker) initialize(id int) {
	st.id = id
	st.tasksChan = make(chan *task)
	st.actionsChan = make(chan action)
	st.closeChan = make(chan chan struct{})
	st.closeListenChan = make(chan struct{})
	st.internalActions = make(map[string]workerInternalActionFunc)
	st.externalActions = make(map[string]workerExternalActionFunc)
	// closed
	st.setStatus(WorkerClosed, false)
	// setup the internal actions' functions
	st.setupInternalFunctionOperations()
}

// GetID returns the ID
func (st *worker) GetID() int {
	return st.id
}

// *************************************************************************************************************
// Channels  ***************************************************************************************************
// *************************************************************************************************************

// GetTasksChannel returns tasks' channel
func (st *worker) GetTasksChannel() chan *task {
	return st.tasksChan
}

// GetActionsChannel returns actions' channel
func (st *worker) GetActionsChannel() chan action {
	return st.actionsChan
}

// Close finalizes the worker: releases all worker's resources.
// If Listen is running it will be exited with no ability to be executed again.
// It returns a channel to let know when Listen is closed.
func (st *worker) Close() chan struct{} {
	// check whether Listen is in progress

	waitingForListenDown := make(chan struct{})
	select {
	// send the chanel to receive a signal once Listen is being stopped :waitingForListenDown
	case st.closeChan <- waitingForListenDown:
		// wait until Listen is down
		<-waitingForListenDown
	default:
		close(waitingForListenDown)
	}

	// close the channels
	close(st.closeChan)
	close(st.GetTasksChannel())
	close(st.GetActionsChannel())

	// flag the worker as "closed"
	st.setStatus(WorkerClosed, true)

	// channel to let know Listen is not longer listening
	return st.closeListenChan
}

// *************************************************************************************************************
// Error Channel  **********************************************************************************************
// *************************************************************************************************************

// SetErrorChannel sets the channel to send errors
func (st *worker) SetErrorChannel(ch chan *PoolError) {
	st.errorChan = ch
}

// GetErrorChannel returns the error channel
func (st *worker) GetErrorChannel() chan *PoolError {
	return st.errorChan
}

// *************************************************************************************************************
// Internal functions  *****************************************************************************************
// *************************************************************************************************************

// setInternalFunctionForAction sets a internal function to be executed after an action
func (st *worker) setInternalFunctionForAction(actionCodes []string, fn workerInternalActionFunc) {
	for _, actionCode := range actionCodes {
		st.internalActions[actionCode] = fn
	}
}

// *************************************************************************************************************
// External functions  *****************************************************************************************
// *************************************************************************************************************

// SetExternalTaskSuccesses sets the external task successes counter
func (st *worker) SetExternalTaskSuccesses(external goconcurrentcounter.Int) {
	st.externalTaskSuccesses = external
}

// GetExternalTaskSuccesses returns the external task successes counter
func (st *worker) GetExternalTaskSuccesses() goconcurrentcounter.Int {
	return st.externalTaskSuccesses
}

// SetExternalFunctionForAction sets a external function to be executed after an action
func (st *worker) SetExternalFunctionForAction(actionCode string, fn workerExternalActionFunc) {
	st.externalActions[actionCode] = fn
}

// SetExternalFeedbackFunction sets the external function to provide feedback about the action/task processing phase.
// The ExternalFeedbackFunction won't be executed after the worker receives the Close action.
func (st *worker) SetExternalFeedbackFunction(fn workerFeedbackFunc) {
	st.externalFeedbackFunc = fn
}

// SetExternalPreActionFunc sets the external function to be executed just before start processing an action
func (st *worker) SetExternalPreActionFunc(fn workerExternalGeneralFunc) {
	st.externalPreActionFunc = fn
}

// SetExternalPostActionFunc sets the external function to be executed after an action gets processed
func (st *worker) SetExternalPostActionFunc(fn workerExternalGeneralFunc) {
	st.externalPostActionFunc = fn
}

// SetExternalPreTaskFunc sets the function to be executed just before start processing an action
func (st *worker) SetExternalPreTaskFunc(fn workerExternalGeneralFunc) {
	st.externalPreTaskFunc = fn
}

// SetExternalPostTaskFunc sets the function to be executed after an action gets processed
func (st *worker) SetExternalPostTaskFunc(fn workerExternalGeneralFunc) {
	st.externalPostTaskFunc = fn
}

// SetExternalFailFunc sets the external function to be executed once the handler fails
func (st *worker) SetExternalFailFunc(fn workerExternalFailFunc) {
	st.externalFailFunc = fn
}

// GetExternalFailFunc returns the external function to be executed once the handler fails
func (st *worker) GetExternalFailFunc() workerExternalFailFunc {
	return st.externalFailFunc
}

// *************************************************************************************************************
// Process  ****************************************************************************************************
// *************************************************************************************************************

// ProcessTask processes a task
func (st *worker) ProcessTask(tsk *task) error {
	// do not allow nil tasks
	if tsk == nil {
		return newPoolError(ErrorWorkerNilTask, "nil task was received by the worker")
	}

	if tsk.Code == taskRegular && tsk.Func == nil && tsk.CallbackFunc == nil {
		return newPoolError(ErrorWorkerNoTaskFunc, "No function provided to process the task")
	}

	st.tasksChan <- tsk

	return nil
}

// ProcessAction processes an action
func (st *worker) ProcessAction(actn action) {
	st.actionsChan <- actn
}

// *************************************************************************************************************
// Listen  *****************************************************************************************************
// *************************************************************************************************************

// Listen listens to actions/tasks and execute them once received.
//
// What kind of information could be processed by a worker?
//	- actions (priority: 1). Actions get processed before enqueued tasks.
//	- tasks (priority: 2).
//	- late actions (priority: 2). Late actions come in the same queue as tasks, that way it allows to execute actions
//	  just after tasks were processed.
func (st *worker) Listen() {
	if !st.getStatus(ListenInProgress) {
		go func() {
			var (
				keepWorking                 = true
				reEnqueueWorker             = true
				isAction                    = false
				isTask                      = false
				executeExternalFeedbackFunc = true
			)

			// Listen in progress
			st.setStatus(ListenInProgress, true)

			for keepWorking {
				isAction = false
				isTask = false
				executeExternalFeedbackFunc = true

				select {
				case closeChan := <-st.closeChan:
					// break the loop
					keepWorking = false
					// do not execute the external feedback function
					executeExternalFeedbackFunc = false
					// sends back the signal saying: "the worker is being closed, Listen is no longer up"
					defer func() { closeChan <- struct{}{} }()
				case actn := <-st.actionsChan:
					isAction = true
					// external function preAction
					if st.externalPreActionFunc != nil {
						st.externalPreActionFunc()
					}

					// internal function
					if internalFunc, ok := st.internalActions[actn.Code]; ok {
						keepWorking = internalFunc()
					}

					// external function
					if externalFunc, ok := st.externalActions[actn.Code]; ok {
						reEnqueueWorker = externalFunc()
					}

				case tsk := <-st.tasksChan:
					// check whether st.Close was called and st.taskChan is being closed (break if true ...)
					if tsk == nil {
						keepWorking = false
						break
					}

					isTask = true

					// regular task

					switch tsk.Code {
					// regular task
					case taskRegular:
						// external function preTask
						if st.externalPreTaskFunc != nil {
							st.externalPreTaskFunc()
						}

						var success bool
						// execute the task's function (if any)
						if tsk.Func != nil {
							//success = tsk.Func(tsk.Data)
							resultChan := make(chan workerFunctionData, 2)
							// execute data handler
							st.executeFunction(tsk.Func, tsk.Data, resultChan, 0)
							result := <-resultChan
							if result.err == nil {
								// no handler issues
								success = result.result
							} else {
								// handler execution failed !
								// report worker's function error
								if st.GetExternalFailFunc() != nil {
									st.GetExternalFailFunc()(tsk, result.err)
								}

								// handler's failure acts as a false
								success = false
							}

							// adjust metrics based on task's function execution
							// update the worker's metrics
							if success {
								st.taskSuccesses++
								// update the external metrics
								if st.externalTaskSuccesses != nil {
									st.externalTaskSuccesses.Update(1)
								}
							} else {
								st.taskFailures++
								// TODO ::: update the external metrics
							}
						}

						// task's callback
						if tsk.CallbackFunc != nil {
							// TODO ::: catch any panics here
							tsk.CallbackFunc(tsk.Data)
						}

					// late action
					case taskLateAction:
						// get the action
						actn, _ := tsk.Data.(action)

						isAction = true
						// late actions come through the tasks channel to keep the order, but they are actions (no tasks)
						isTask = false

						// execute all functions related to actions
						// external function preAction
						if st.externalPreActionFunc != nil {
							st.externalPreActionFunc()
						}

						// internal function
						if internalFunc, ok := st.internalActions[actn.Code]; ok {
							keepWorking = internalFunc()
						}

						// external function
						if externalFunc, ok := st.externalActions[actn.Code]; ok {
							reEnqueueWorker = externalFunc()
						}
					}

				}

				// keepWorker overwrites reEnqueueWorker whether keepWorker == false
				if !keepWorking {
					reEnqueueWorker = false
				}

				// let the pool's dispatcher knows whether this worker could be re-enqueued (based on reEnqueueWorker)
				if executeExternalFeedbackFunc && st.externalFeedbackFunc != nil {
					// call the external feedback function
					st.externalFeedbackFunc(st.GetID(), reEnqueueWorker)
				}

				// external post action/task function
				// action
				if isAction && st.externalPostActionFunc != nil {
					st.externalPostActionFunc()
					continue
				}
				// task
				if isTask && st.externalPostTaskFunc != nil {
					st.externalPostTaskFunc()
					continue
				}

			}

			// Listen is done, not "in progress" anymore
			st.setStatus(ListenInProgress, false)

			// let know Listen is not longer listening
			select {
			case st.closeListenChan <- struct{}{}:
			default:
				// do nothing, st.closeListenChan is closed
			}
		}()
	}
}

// executeFunction executes the user function, sends the return over resultChan and controls the time execution (optional)
func (st *worker) executeFunction(fn PoolFunc, data interface{}, resultChan chan workerFunctionData, maxDuration time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			resultChan <- workerFunctionData{
				result: false,
				err:    newPoolError(ErrorWorkerFuncPanic, fmt.Sprintf("%v", r)),
			}
		}
	}()

	// no max execution time
	if maxDuration == 0 {
		resultChan <- workerFunctionData{
			result: fn(data),
			err:    nil,
		}
	} else {
		doneChan := make(chan bool)
		go func(fn PoolFunc, data interface{}, doneChan chan bool) {
			// TODO ::: defer && recover
			doneChan <- fn(data)
		}(fn, data, doneChan)

		select {
		case result := <-doneChan:
			resultChan <- workerFunctionData{
				result: result,
				err:    nil,
			}
		case <-time.After(maxDuration):
			resultChan <- workerFunctionData{
				result: false,
				err:    newPoolError(ErrorWorkerMaxTimeExceeded, "Function exceeded max execution time"),
			}
		}
	}
}

// *************************************************************************************************************
// ** Metrics  *************************************************************************************************
// *************************************************************************************************************

// GetTaskSuccesses returns how many tasks has been successfully processed
func (st *worker) GetTaskSuccesses() int {
	return st.taskSuccesses
}

// GetTaskFailures returns how many tasks has not been successfully processed
func (st *worker) GetTaskFailures() int {
	return st.taskFailures
}

// *************************************************************************************************************
// ** Statuses. Operation in progress  *************************************************************************
// *************************************************************************************************************

// setStatus sets the status for a given operation
func (st *worker) setStatus(operationName string, status bool) {
	st.generalInfoMap.Store(operationName, status)
}

// getStatus returns the status for a given operation
func (st *worker) getStatus(operationName string) bool {
	rawValue, ok := st.generalInfoMap.Load(operationName)
	if !ok {
		return false
	}

	value := rawValue.(bool)
	return value
}

// Closed returns true if the worker was closed using Close
func (st *worker) Closed() bool {
	return st.getStatus(WorkerClosed)
}

// *************************************************************************************************************
// ** Internal action operations  ******************************************************************************
// *************************************************************************************************************

func (st *worker) setupInternalFunctionOperations() {
	// KillWorker
	st.setInternalFunctionForAction([]string{actionKillWorker, actionLateKillWorker}, func() bool {
		// return false would break the Listen loop, so the worker will die
		return false
	})
}
