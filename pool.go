// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"fmt"
	"sync"

	"github.com/enriquebris/goconcurrentcounter"
	"github.com/enriquebris/goconcurrentqueue"
)

const (
	// Statuses
	// Wait in-progress status
	waitInProgress = "wait.in.progress"
	// KillAllWorkers in-progress status
	killAllWorkersInProgress = "kill.all.workers.in.progress"
	// is dispatcher alive? status
	dispatcherInProgress = "dispatcher.in.progress"

	triggerZeroTotalWorkers    = "trigger.zero.total.workers"
	triggerWaitUntilNSuccesses = "trigger.wait.until.n.successes"

	// max amount of possible workers
	defaultMaxWorkers = 100
)

// PoolFunc defines the function signature to be implemented by the workers
type PoolFunc func(interface{}) bool

// PoolCallback defines the callback function signature
type PoolCallback func(interface{})

// dispatcherGeneralFunc function to be executed by the dispatcher (could be part of an action)
type dispatcherGeneralFunc func()

type Pool struct {
	// tasks' queue
	tasks goconcurrentqueue.Queue
	// tasks that panicked at the worker
	panicTasks goconcurrentqueue.Queue
	// actions' queue
	actions goconcurrentqueue.Queue
	// map of workers
	workers sync.Map
	// available workers (available means idle)
	availableWorkers goconcurrentqueue.Queue

	// default worker's function
	defaultWorkerFunc PoolFunc

	// channel to enqueue a signal if a task or action was enqueued
	activityChan chan struct{}
	// channel to send worker's feedback to let dispatcher knows that the worker should be re-enqueued into availableWorkers
	dispatcherWorkerFeedbackChan chan workerFeedback
	// channel to let the dispatcher knows whether the workers could be resumed
	resumeWorkers chan struct{}

	// channel to enqueue a signal if all workers were killed
	noWorkersChan chan struct{}
	// channel to send a signal once initial amount of workers are up
	waitUntilInitialWorkersAreUp chan struct{}

	// next available ID for new worker
	newWorkerID goconcurrentcounter.Int

	// metrics
	// total workers
	totalWorkers goconcurrentcounter.Int
	// to keep track the successful tasks
	taskSuccesses goconcurrentcounter.Int
	// amount of workers processing actions/tasks
	totalWorkersInProgress goconcurrentcounter.Int

	// external channels
	// to send a signal to an external listener once a new worker is created
	externalNewWorkerChan chan int
	// to send a signal to an external listener once a worker is terminated
	externalKilledWorkerChan chan int

	// log
	logVerbose bool
	logChan    chan PoolLog

	// general information map (actions in progress, etc)
	generalInfoMap sync.Map
}

// NewPool is deprecated since version 0.10.0, please do not use it. Use NewPoolWithOptions instead.
// NewPool builds and returns a new Pool.
// This function will be removed on version 1.0.0
// Parameters:
//	initialWorkers = amount of workers to start up at initialization
//	maxOperationsInQueue = maximum amount of actions/tasks that could be enqueued at the same time
//	logVerbose = true ==> output logs
func NewPool(initialWorkers int, maxOperationsInQueue int, logVerbose bool) *Pool {
	// wrong data entry
	if initialWorkers < 0 || maxOperationsInQueue <= 0 {
		return nil
	}

	ret := &Pool{}
	ret.initialize(PoolOptions{
		TotalInitialWorkers:          uint(initialWorkers),
		MaxWorkers:                   defaultMaxWorkers,
		MaxOperationsInQueue:         uint(maxOperationsInQueue),
		WaitUntilInitialWorkersAreUp: false,
		LogVerbose:                   logVerbose,
	})

	return ret
}

// NewPoolWithOptions builds and returns a Pool.
// This function will return error if any of the following scenarios occurs:
//	- MaxWorkers == 0
//	- MaxOperationsInQueue == 0
func NewPoolWithOptions(options PoolOptions) (*Pool, error) {
	// validate entry values
	// MaxWorkers
	if options.MaxWorkers == 0 {
		return nil, newPoolError(ErrorData, "MaxWorkers should be greater than zero")
	}
	// MaxOperationsInQueue
	if options.MaxOperationsInQueue == 0 {
		return nil, newPoolError(ErrorData, "MaxOperationsInQueue should be greater than zero")
	}

	pool := &Pool{}
	// new worker channel
	pool.SetNewWorkerChan(options.NewWorkerChan)

	//pool.initialize(int(options.TotalInitialWorkers), int(options.MaxOperationsInQueue), int(options.MaxWorkers))
	pool.initialize(options)

	if options.WaitUntilInitialWorkersAreUp {
		// wait until all initial workers are up and running
		<-pool.waitUntilInitialWorkersAreUp
	}

	return pool, nil
}

// initialize initializes the Pool
func (st *Pool) initialize(options PoolOptions) {
	st.tasks = goconcurrentqueue.NewFIFO()
	st.panicTasks = goconcurrentqueue.NewFIFO()
	st.actions = goconcurrentqueue.NewFIFO()
	st.availableWorkers = goconcurrentqueue.NewFIFO()

	// initialize channels
	st.activityChan = make(chan struct{}, int(options.MaxOperationsInQueue))
	st.dispatcherWorkerFeedbackChan = make(chan workerFeedback, int(options.MaxWorkers))
	st.noWorkersChan = make(chan struct{})
	st.waitUntilInitialWorkersAreUp = make(chan struct{}, 2)

	// initialize newWorkerID (0)
	st.newWorkerID = goconcurrentcounter.NewIntChan(0)
	// initialize totalWorkers
	st.totalWorkers = goconcurrentcounter.NewIntChan(0)

	// add a trigger function to send a signal once all initial workers are up
	st.totalWorkers.SetTriggerOnValue(int(options.TotalInitialWorkers), "WaitUntilInitialWorkersAreUp", func() {
		// remove the trigger function
		st.totalWorkers.EnqueueToRunAfterCurrentTriggerFunctions(func() {
			st.totalWorkers.UnsetTriggerOnValue(int(options.TotalInitialWorkers), "WaitUntilInitialWorkersAreUp")
		})

		// send the signal
		st.waitUntilInitialWorkersAreUp <- struct{}{}
		if options.WaitUntilInitialWorkersAreUp {
			// send an extra signal to the NewPoolWithOptions function
			st.waitUntilInitialWorkersAreUp <- struct{}{}
		}
		close(st.waitUntilInitialWorkersAreUp)
	})

	// metrics
	// zero successful tasks at initialization
	st.taskSuccesses = goconcurrentcounter.NewIntChan(0)
	// total workers in progress
	st.totalWorkersInProgress = goconcurrentcounter.NewIntChan(0)

	// start running the dispatcher in a separate GR
	go st.dispatcher()
	// start running the dispatcherWorkerFeedback in a separate GR
	go st.dispatcherWorkerFeedback()

	// initialize workers
	for i := 0; i < int(options.TotalInitialWorkers); i++ {
		if err := st.AddWorker(); err != nil {
			// log if the following action fails
			st.logError("initialize.AddWorker", err)
		}
	}

	// send signal if totalWorkers == 0 (all workers are down)
	st.totalWorkers.SetTriggerOnValue(0, triggerZeroTotalWorkers, func() {
		// remove the "killAllWorkersInProgress" flag (if active)
		if st.getStatus(killAllWorkersInProgress) {
			st.setStatus(killAllWorkersInProgress, false)
		}

		// sends only the signal if Wait() was invoked and it is running
		if st.getStatus(waitInProgress) {
			st.getNoWorkersChan() <- struct{}{}
		}
	})
}

// *************************************************************************************************************
// ** Dispatcher  **********************************************************************************************
// *************************************************************************************************************

// dispatcher keeps listening activityChan and dispatches tasks and actions
func (st *Pool) dispatcher() {
	keepWorking := true
	for keepWorking {
		var (
			noAction     = true
			noTask       = true
			actn         action
			tsk          *task
			preFunc      dispatcherGeneralFunc = nil
			sendToWorker bool                  = true
		)

		// flag: "dispatcher is alive"
		st.setStatus(dispatcherInProgress, true)

		select {
		case _, ok := <-st.activityChan:
			if !ok {
				// st.activityChan is closed
				keepWorking = false
				break
			}

			// check actions
			if rawAction, err := st.actions.Dequeue(); err != nil {
				queueError, ok := err.(*goconcurrentqueue.QueueError)
				if ok && queueError.Code() == goconcurrentqueue.QueueErrorCodeEmptyQueue {
					noAction = true
				} else {
					// error at action dequeue
					st.logError("dispatcher.action.dequeue", err)
					continue
				}
			} else {
				noAction = false
				actn, _ = rawAction.(action)
				// get the action's function
				preFunc = actn.PreExternalFunc
				// get the action's SendToWorker (it defines whether the action should be sent to the worker or executed by the dispatcher)
				sendToWorker = actn.SendToWorker
			}

			if noAction {
				// check tasks / jobs
				if rawTask, err := st.tasks.Dequeue(); err != nil {
					queueError, ok := err.(*goconcurrentqueue.QueueError)

					if ok && queueError.Code() == goconcurrentqueue.QueueErrorCodeEmptyQueue {
						// no new task to process
						noTask = true
					} else {
						// error at task dequeue
						st.logError("dispatcher.task.dequeue", err)
						continue
					}
				} else {
					noTask = false
					tsk, _ = rawTask.(*task)

					// check whether a late action (coming as a task) should be processed by the dispatcher (not by the worker)
					if tsk.Code == taskLateAction {
						// even knowing that the task's data contains an action, it should be sent as a task, later the
						// worker would transform the data

						tmpAction, ok := tsk.Data.(action)
						if !ok {
							st.logError("dispatcher.taskLateAction unexpected type", nil)
							continue
						}

						// get the action's SendToWorker (it defines whether the action should be sent to the worker or executed by the dispatcher)
						sendToWorker = tmpAction.SendToWorker
						// get the action's function
						preFunc = tmpAction.PreExternalFunc
					}
				}
			}

			if noAction && noTask {
				// return a unexpected error: the activityChan received a signal but there isn't an action nor a task
				st.logError("no action/task at dispatcher.dequeue", newPoolError(ErrorDispatcherNoActionNoTask, "no action/task at dispatcher.dequeue"))
				continue
			} else {
				// check whether a function should be executed
				if preFunc != nil {
					preFunc()
				}

				// check whether the data should be sent to a worker
				if !sendToWorker {
					// do not send the data to a worker
					continue
				}

				// now we have the action/task, so let's get the next available worker to process it
				if rawWorker, err := st.availableWorkers.DequeueOrWaitForNextElement(); err != nil {
					st.logError("dispatcher.availableWorker.dequeue", err)

					// as the worker dequeue failed, then reEnqueue the action/job
					if noAction {
						// re-enqueue the task
						if err := st.AddTask(tsk); err != nil {
							st.logError("dispatcher.reEnqueue.AddTask", err)
						}
					} else {
						// re-enqueue the action
						if err := st.addAction(actn.Code, actn.SendToWorker, actn.PreExternalFunc); err != nil {
							st.logError("dispatcher.reEnqueue.AddAction", err)
						}
					}
					continue
				} else {
					// there is no need to verify the *worker type
					wkr, _ := rawWorker.(*worker)

					if noTask {
						// process the action
						wkr.ProcessAction(actn)
					} else {
						// TODO ::: catch errors here, send them by the errors channel
						// process the task
						wkr.ProcessTask(tsk)
					}

					// The worker would be re-enqueued into availableWorkers based on the feedback sent after the processing
					// phase. This feedback would be sent directly from the same worker to the pool's function: dispatcherWorkerFeedback

				}
			}
		}

	}

	// flag: "dispatcher is no longer alive"
	st.setStatus(dispatcherInProgress, false)
}

// dispatcherWorkerFeedback keeps listening the dispatcherWorkerFeedbackChan to re-enqueue workers in availableWorkers
func (st *Pool) dispatcherWorkerFeedback() {
	keepWorking := true
	for keepWorking {
		select {
		case workerFeedback := <-st.dispatcherWorkerFeedbackChan:
			// TODO ::: send the worker's object from the worker instead of the workerID and measure how faster it is, and how much memory is involved.
			// this way it avoids to do the st.workers.Load(workerFeedback.workerID) and stops the execution because the st.workers lock !!!

			// get the worker
			//wkr := workerFeedback.wkr

			rawWorker, ok := st.workers.Load(workerFeedback.workerID)
			if !ok {
				st.logError("dispatcherWorkerFeedback.unknown worker", newPoolError(ErrorWorkerNotFound, fmt.Sprintf("Missing worker %v could not be removed", workerFeedback.workerID)))
				continue
			}
			wkr, ok := rawWorker.(*worker)
			if !ok {
				st.logError("dispatcherWorkerFeedback.worker", newPoolError(ErrorWorkerType, fmt.Sprintf("Wrong worker's type for id: %v", workerFeedback.workerID)))
				continue
			}

			if workerFeedback.reEnqueueWorker {
				// try to re-enqueue it
				if err := st.availableWorkers.Enqueue(wkr); err != nil {
					st.logError("dispatcherWorkerFeedback.availableWorkers.Enqueue", err)
				} else {
					continue
				}
			}

			// remove the worker
			if err := st.removeWorker(wkr.GetID()); err != nil {
				st.logError("dispatcherWorkerFeedback.removeWorker", err)
			}

			// TODO ::: keep track of the "not" re-enqueued workers
		}
	}
}

// *************************************************************************************************************
// ** Tasks/Actions operations *********************************************************************************
// *************************************************************************************************************

// addTask enqueues a task / job
func (st *Pool) addTask(taskType int, data interface{}, fn PoolFunc, callbackFn PoolCallback) error {
	job := &task{
		Code:         taskType,
		Data:         data,
		Func:         fn,
		CallbackFunc: callbackFn,
		Valid:        true,
	}

	// enqueue the regular task
	if err := st.tasks.Enqueue(job); err != nil {
		return err
	}

	// send a signal over doSomethingChan (to let operationsHandler knows that there is "something" to do)
	select {
	case st.activityChan <- struct{}{}:
		// do nothing, the signal was successfully sent
	default:
		// the channel is full so the signal couldn't be enqueued

		// set the job as invalid ==> if a worker picks it will be skipped without further processing
		job.Valid = false

		return newPoolError(ErrorDispatcherChannelFull, "task couldn't be enqueued because the operations(tasks/actions) queue is at full capacity")
	}

	return nil
}

// addAction enqueue an action
func (st *Pool) addAction(actionCode string, sendToWorker bool, preExternalFunc dispatcherGeneralFunc) error {
	actn := action{
		Code:            actionCode,
		SendToWorker:    sendToWorker,
		PreExternalFunc: preExternalFunc,
	}

	if err := st.actions.Enqueue(actn); err != nil {
		return err
	}

	// send a signal over activityChan (to let operationsHandler knows that there is "something" to do)
	select {
	case st.activityChan <- struct{}{}:
		// do nothing, the signal was successfully sent
	default:
		// the channel is full so the signal couldn't be enqueued
		return newPoolError(ErrorDispatcherChannelFull, "action couldn't be enqueued because the operations(tasks/actions) queue is at full capacity")
	}

	return nil
}

// AddTask enqueues a task (into a FIFO queue).
//
// The parameter for the task's data accepts any type (interface{}).
//
// Workers (if any) will be listening to and picking up tasks from this queue. If no workers are alive nor idle,
// the task will stay in the queue until any worker will be ready to pick it up and start processing it.
//
// The queue in which this function enqueues the tasks has a limit (it was set up at pool initialization). It means that AddTask will wait
// for a free queue slot to enqueue a new task in case the queue is at full capacity.
//
// AddTask returns an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
// a certain amount of time when WaitUntilNSuccesses meets the stop condition, or in other words: when KillAllWorkers is
// in progress.
func (st *Pool) AddTask(data interface{}) error {
	// verify KillAllWorkers operation is not in progress
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}
	// verify worker default function handler
	if st.defaultWorkerFunc == nil {
		return newPoolError(ErrorNoDefaultWorkerFunction, "Missing default handler function to process this task")
	}

	return st.addTask(taskRegular, data, st.defaultWorkerFunc, nil)
}

// AddTaskCallback enqueues a job and a callback function into a FIFO queue.
//
// The parameter for the job's data accepts any data type (interface{}).
//
// Workers will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
// the job will stay in the queue until any worker will be ready to pick it up and start processing it.
//
// The worker who picks up this job and callback will process the job first and later will invoke the callback function,
// passing the job's data as a parameter.
//
// The queue in which this function enqueues the jobs has a capacity limit (it was set up at pool initialization). This
// means that AddTaskCallback will wait for a free queue slot to enqueue a new job in case the queue is at full capacity.
//
// AddTaskCallback will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
// a certain amount of time when WaitUntilNSuccesses meets the stop condition.
func (st *Pool) AddTaskCallback(data interface{}, callbackFn PoolCallback) error {
	// verify callbackFn
	if callbackFn == nil {
		return newPoolError(ErrorData, "Callback function should not be nil")
	}

	// verify KillAllWorkers operation is not in progress
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// verify worker default function handler
	if st.defaultWorkerFunc == nil {
		return newPoolError(ErrorNoDefaultWorkerFunction, "Missing default handler function to process this task")
	}

	return st.addTask(taskRegular, data, st.defaultWorkerFunc, callbackFn)
}

// AddCallback enqueues a callback function into a FIFO queue.
//
// Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
// the job will stay in the queue until any worker will be ready to pick it up and start processing it.
//
// The worker who picks up this job will only invoke the callback function, passing nil as a parameter.
//
// The queue in which this function enqueues the jobs has a limit (it was set up at pool initialization). It means that AddCallback will wait
// for a free queue slot to enqueue a new job in case the queue is at full capacity.
//
// AddCallback will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
// a certain amount of time when WaitUntilNSuccesses meets the stop condition.
func (st *Pool) AddCallback(callbackFn PoolCallback) error {
	// verify callbackFn
	if callbackFn == nil {
		return newPoolError(ErrorData, "Callback function should not be nil")
	}

	// verify KillAllWorkers operation is not in progress
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// TODO ::: why do we need a defaultWorkerFunc to execute the callback ???
	// verify worker default function handler
	if st.defaultWorkerFunc == nil {
		return newPoolError(ErrorNoDefaultWorkerFunction, "Missing default handler function to process this task")
	}

	return st.addTask(taskRegular, nil, nil, callbackFn)
}

// *************************************************************************************************************
// ** Worker's operations  *************************************************************************************
// *************************************************************************************************************

// StartWorkers is deprecated since version 0.10.0, please do not use it.
// Why is StartWorkers deprecated? There is no need to explicitly starts up the workers as they automatically
// start up once created.
//
// For backward compatibility, this function only returns nil.
// StartWorkers will be removed on version 1.0.0
//
// Prior to version 0.10.0, the goal for this function was to start up the amount of workers pre-defined at pool instantiation.
func (st *Pool) StartWorkers() error {
	return nil
}

// StartWorkersAndWait is deprecated since version 0.10.0, please do not use it.
// Use WaitUntilInitialWorkersAreUp instead.
// Why is StartWorkersAndWait deprecated? Workers are started up once they get created, there is no need to do it explicitly.
//
// For backward compatibility, this function will return WaitUntilInitialWorkersAreUp.
// StartWorkersAndWait will be removed on version 1.0.0
//
// Prior to version 0.10.0, the goal of this function was to start up all workers pre-defined at pool instantiation
// and wait until until all them were up and running.
func (st *Pool) StartWorkersAndWait() error {
	return st.WaitUntilInitialWorkersAreUp()
}

// SetWorkerFunc sets the worker's default function handler.
// This function will be invoked each time a worker pulls a new job, and should return true to let know that the job
// was successfully completed, or false in other case.
func (st *Pool) SetWorkerFunc(fn PoolFunc) {
	st.defaultWorkerFunc = fn
}

// getNoWorkersChan returns the channel over which the "no workers" signal travels
func (st *Pool) getNoWorkersChan() chan struct{} {
	return st.noWorkersChan
}

// getNewWorkerID returns an incremental ID for the next worker to be created.
// This is a concurrent-safe function.
func (st *Pool) getNewWorkerID() int {
	defer st.newWorkerID.Update(1)

	return st.newWorkerID.GetValue()
}

// AddWorker adds a new worker to the pool.
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) could be set using SetNewWorkerChan(chan) or at initialization (PoolOptions.NewWorkerChan in NewPoolWithOptions).
//
// AddWorker returns an error if at least one of the following statements is true:
//  - the worker could not be started
//  - there is a "in course" KillAllWorkers operation
func (st *Pool) AddWorker() error {
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// build the worker
	wkr := newWorker(st.getNewWorkerID())
	// save new worker into the map
	st.workers.Store(wkr.GetID(), wkr)
	// setup action external functions
	st.setupWorkerActionExternalFunctions(wkr)
	// increment the totalWorkers counter
	st.totalWorkers.Update(1)

	// add the new worker into the available workers list
	if err := st.availableWorkers.Enqueue(wkr); err != nil {
		// TODO ::: verify the following log in the testings
		st.logError("AddWorker.availableWorkers.Enqueue", err)
		if errRemoveWorker := st.removeWorker(wkr.GetID()); errRemoveWorker != nil {
			st.logError("AddWorker.removeWorker", errRemoveWorker)
		}
		return err
	}

	// setup the new worker (add all external dependencies)
	st.setupWorker(wkr)

	// start listening requests (actions / tasks)
	wkr.Listen()

	// notify the user through the pre-configured external channel
	if st.externalNewWorkerChan != nil {
		// non-blocking operation
		select {
		case st.externalNewWorkerChan <- 1:
		default:
			// signal couldn't be sent over the external channel
			st.logError("AddWorker.externalNewWorkerChan - signal couldn't be sent over the external channel once a new worker was created", newPoolError(ErrorFullCapacityChannel, "AddWorker.externalNewWorkerChan - signal couldn't be sent over the external channel once a new worker was created"))
		}
	}

	return nil
}

// AddWorkers adds n extra workers to the pool.
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) could be set using SetNewWorkerChan(chan) or at initialization (PoolOptions.NewWorkerChan in NewPoolWithOptions).
//
// AddWorkers returns an error if at least one of the following statements are true:
//	- parameter n <= 0
//  - a worker could not be started
//  - there is a "in course" KillAllWorkers operation
func (st *Pool) AddWorkers(n int) error {
	if n <= 0 {
		return newPoolError(ErrorData, "Amount of workers should be greater than zero")
	}

	errors := make([]error, 0)
	for i := 0; i < n; i++ {
		if err := st.AddWorker(); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		err := newPoolError(ErrorEmbeddedErrors, "Some internal AddWorkers' operations failed.")
		err.AddEmbeddedErrors(errors)

		return err
	}

	return nil
}

// setupWorker sets up the worker:
//  - adds all external dependencies:
//		- external task successes
//		- external function for actions/tasks
func (st *Pool) setupWorker(wkr *worker) {
	// link the taskSuccesses counter with the new worker
	wkr.SetExternalTaskSuccesses(st.taskSuccesses)

	// external functions for actions/tasks
	// function to be executed just before start processing a task
	wkr.SetExternalPreTaskFunc(func() {
		st.totalWorkersInProgress.Update(1)
	})
	// function to be executed after a task gets processed (by the worker)
	wkr.SetExternalPostTaskFunc(func() {
		st.totalWorkersInProgress.Update(-1)
	})
	// function to be executed just before start processing an action (by the worker)
	wkr.SetExternalPreActionFunc(func() {
		st.totalWorkersInProgress.Update(1)
	})
	// function to be executed after an action gets processed
	wkr.SetExternalPostActionFunc(func() {
		st.totalWorkersInProgress.Update(-1)
	})
}

// removeWorker closes and removes a worker (from the workers' map)
// note: this function doesn't remove the worker from the availableWorkers queue
func (st *Pool) removeWorker(workerID int) error {
	//st.availableWorkers.Lock()
	//defer st.availableWorkers.Unlock()

	if rawWkr, ok := st.workers.Load(workerID); ok {

		// remove the workers entry
		st.workers.Delete(workerID)
		// decrement the workers counter
		st.totalWorkers.Update(-1)

		wkr, okTypeAssertion := rawWkr.(*worker)
		if !okTypeAssertion {
			return newPoolError(ErrorWorkerType, fmt.Sprintf("worker %v could not be removed due to type error", workerID))
		}
		// close the worker
		wkr.Close()

		// notify the user through the pre-configured external channel
		if st.externalKilledWorkerChan != nil {
			// non-blocking operation
			select {
			case st.externalKilledWorkerChan <- 1:
			default:
				// signal couldn't be sent over the external channel
				st.logError("removeWorker.externalKilledWorkerChan - signal couldn't be sent over the external channel once a worker was terminated", nil)
			}
		}

		return nil
	}

	return newPoolError(ErrorWorkerNotFound, fmt.Sprintf("Missing worker %v could not be removed", workerID))
}

// SetTotalWorkers adjusts the number of live workers.
//
// In case it needs to kill some workers (in order to adjust the total based on the given parameter), it will wait until
// their current jobs get processed (in case they are processing jobs).
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) can be defined at SetNewWorkerChan(chan).
//
// SetTotalWorkers returns an error in the following scenarios:
//  - There is a "in course" KillAllWorkers operation.
func (st *Pool) SetTotalWorkers(n int) error {
	if n < 0 {
		return newPoolError(ErrorData, "total workers must be >= 0")
	}

	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	totalWorkers := st.GetTotalWorkers()

	// n < totalWorkers
	if n < totalWorkers {
		return st.KillWorkers(totalWorkers - n)
	}
	// n > totalWorkers
	if n > totalWorkers {
		return st.AddWorkers(n - totalWorkers)
	}

	// n == totalWorkers, no changes needed
	return nil
}

// *************************************************************************************************************
// ** Worker's operations: kill && lateKill  *******************************************************************
// *************************************************************************************************************

// KillWorker kills an idle worker.
// The kill signal has a higher priority than the enqueued jobs. It means that a worker will be killed once it finishes its current job although there are unprocessed jobs in the queue.
// Use LateKillWorker() in case you need to wait until current enqueued jobs get processed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) KillWorker() error {
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	return st.killWorker()
}

// killWorker sends a signal to kill the next available(idle) worker
func (st *Pool) killWorker() error {
	return st.addAction(actionKillWorker, true, nil)
}

// KillWorkers kills n idle workers.
// If n > GetTotalWorkers(), only current amount of workers will be terminated.
// The kill signal has a higher priority than the enqueued jobs. It means that a worker will be killed once it finishes its current job, no matter if there are unprocessed jobs in the queue.
// Use LateKillAllWorkers() ot LateKillWorker() in case you need to wait until current enqueued jobs get processed.
// It returns an error in the following scenarios:
//  - there is a "in course" KillAllWorkers operation
//  -
func (st *Pool) KillWorkers(n int) error {
	if n <= 0 {
		return newPoolError(ErrorData, "Incorrect number of workers")
	}

	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	errors := make([]error, 0)
	totalWorkers := st.GetTotalWorkers()
	// adjust n
	if n > totalWorkers {
		n = totalWorkers
	}

	for i := 0; i < n; i++ {
		if err := st.killWorker(); err != nil {
			// count how many attempts failed
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		// return a generic error containing single errors
		err := newPoolError(ErrorEmbeddedErrors, "Some internal KillWorkers' operations failed.")
		err.AddEmbeddedErrors(errors)

		return err
	}

	return nil
}

// KillAllWorkers kills all live workers (the number of live workers is determined at the moment this action is processed).
// If a worker is processing a job, it will not be immediately killed, the pool will wait until the current job gets processed.
//
// The following functions will return error if invoked during KillAllWorkers execution:
//
//  - KillWorker
//  - KillWorkers
//  - LateKillWorker
//  - LateKillWorkers
//  - LateKillAllWorkers
//  - AddWorker
//  - AddWorkers
//  - SetTotalWorkers
func (st *Pool) KillAllWorkers() error {
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, "KillAllWorkers already in progress")
	}

	// this flag will be removed once all workers were down
	st.setStatus(killAllWorkersInProgress, true)

	// 1 - get total workers
	totalWorkers := st.GetTotalWorkers()
	embeddedErrors := make([]error, 0)
	// 2 - send "kill worker" message to all workers
	for i := 0; i < totalWorkers; i++ {
		if err := st.killWorker(); err != nil {
			embeddedErrors = append(embeddedErrors, err)
		}
	}

	if len(embeddedErrors) > 0 {
		err := newPoolError(ErrorEmbeddedErrors, "Some internal KillAllWorkers' operations failed")
		err.AddEmbeddedErrors(embeddedErrors)
		return err
	}

	return nil
}

// KillAllWorkersAndWait kills all workers (the amount of workers is determined at the moment this action is processed).
// This function waits until current alive workers are down. This is the difference between KillAllWorkersAndWait() and KillAllWorkers()
// If a worker is processing a job, it will not be immediately terminated, the pool will wait until the current job gets processed.
//
// The following functions will return error if invoked during this function execution:
//
//  - KillWorker
//  - KillWorkers
//  - LateKillWorker
//  - LateKillWorkers
//  - LateKillAllWorkers
//  - AddWorker
//  - AddWorkers
//  - SetTotalWorkers
func (st *Pool) KillAllWorkersAndWait() error {
	// kill all workers
	if err := st.KillAllWorkers(); err != nil {
		return err
	}

	// wait until all workers are down
	waitForKillAllWorkersAndWait := make(chan struct{}, 1)
	//	1 - trigger a named function on totalWorkers (0)
	st.totalWorkers.SetTriggerOnValue(0, "KillAllWorkersAndWait", func() {
		// enqueue to run after trigger functions
		st.totalWorkers.EnqueueToRunAfterCurrentTriggerFunctions(func() {
			//	remove the named trigger
			st.totalWorkers.UnsetTriggerOnValue(0, "KillAllWorkersAndWait")
		})

		// it's done, send the signal
		waitForKillAllWorkersAndWait <- struct{}{}
	})

	<-waitForKillAllWorkersAndWait
	return nil
}

// LateKillWorker kills a worker only after current enqueued jobs get processed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) LateKillWorker() error {
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// enqueues the action as a task because it is a "late" action that would be executed after the current enqueued tasks
	return st.addTask(taskLateAction, action{
		Code:         actionLateKillWorker,
		SendToWorker: true,
	}, nil,
		nil)
}

// LateKillWorkers kills n workers only after all current jobs get processed.
// If n > GetTotalWorkers(), only current amount of workers will be terminated.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) LateKillWorkers(n int) error {
	if n <= 0 {
		return newPoolError(ErrorData, "Incorrect number of workers")
	}

	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// Enqueues an action as a late task. This action (the PreExternalFunc) would be executed by the dispatcher, not by
	// the workers (SendToWorker==false).
	return st.addTask(
		taskLateAction,
		action{
			Code:         actionLateKillWorkers,
			SendToWorker: false,
			PreExternalFunc: func() {
				totalWorkers := st.GetTotalWorkers()
				// adjust n based on current amount of workers
				if n > totalWorkers {
					n = totalWorkers
				}

				if err := st.KillWorkers(n); err != nil {
					st.logError("LateKillWorkers.KillWorkers", err)
				}
			},
		},
		nil,
		nil)
}

// LateKillAllWorkers kills all live workers only after all current jobs get processed.
// By "current jobs" it means: the number of enqueued jobs in the exact moment this function get executed.
// It returns an error for the following scenarios:
//	- there is a "in course" KillAllWorkers operation.
//	- the operations' queue (where tasks/actions get enqueued) is at full capacity
func (st *Pool) LateKillAllWorkers() error {
	if st.getStatus(killAllWorkersInProgress) {
		return newPoolError(ErrorKillAllWorkersInProgress, messageKillAllWorkersInProgress)
	}

	// Enqueues an action as a late task. This action (the PreExternalFunc) would be executed by the dispatcher, not by
	// the workers (SendToWorker==false).
	return st.addTask(
		taskLateAction,
		action{
			Code:         actionLateKillAllWorkers,
			SendToWorker: false,
			PreExternalFunc: func() {
				st.KillAllWorkers()
			},
		},
		nil,
		nil)
}

// *************************************************************************************************************
// ** Wait functions  ******************************************************************************************
// *************************************************************************************************************

// Wait waits while at least a worker is up and running
func (st *Pool) Wait() error {
	if st.totalWorkers.GetValue() == 0 {
		return nil
	}

	if st.getStatus(waitInProgress) {
		return newPoolError(ErrorWaitInProgress, "Wait function is already in progress")
	}

	// flag to know that Wait is in progress and can't be executed again until it finishes
	st.setStatus(waitInProgress, true)

	// wait until all workers are down
	<-st.getNoWorkersChan()
	st.setStatus(waitInProgress, false)

	return nil
}

// WaitUntilNSuccesses waits until n workers finished their job successfully, then kills all active workers.
// A worker is considered successfully if the associated worker function returned true.
// TODO ::: An error will be returned if the worker's function is not already set.
func (st *Pool) WaitUntilNSuccesses(n int) error {
	wait := make(chan struct{})
	st.taskSuccesses.SetTriggerOnValue(n, triggerWaitUntilNSuccesses, func() {
		// kill all workers
		st.KillAllWorkers()

		// execute just after the trigger functions
		st.taskSuccesses.EnqueueToRunAfterCurrentTriggerFunctions(func() {
			// this function should be executed only once, so let's remove it after it's first call
			st.taskSuccesses.UnsetTriggerOnValue(n, triggerWaitUntilNSuccesses)
		})

		// termination signal
		wait <- struct{}{}
	})

	<-wait
	return nil
}

// SafeWaitUntilNSuccesses waits until n workers finished their job successfully, then kills all active workers.
// A job/task is considered successful if the worker that processed it returned true.
// It could happen that other workers were processing jobs at the time the nth job was successfully processed, so this
// function will wait until all those extra workers were done. In other words, SafeWaitUntilNSuccesses guarantees that
// when it finishes no worker would be processing data.
// TODO ::: this function fails if n >= total workers
func (st *Pool) SafeWaitUntilNSuccesses(n int) error {
	// channel to wait until n successes
	wait := make(chan struct{})
	// channel to wait until all extra workers finished their processing
	safeWait := make(chan struct{})

	// to be executed once extra workers finished their processing
	st.totalWorkersInProgress.SetTriggerOnValue(0, "safeSafeWaitUntilNSuccesses", func() {
		// execute just after the trigger functions
		st.totalWorkersInProgress.EnqueueToRunAfterCurrentTriggerFunctions(func() {
			// this function should be executed only once, let's remove it after the first call
			st.totalWorkersInProgress.UnsetTriggerOnValue(0, "safeSafeWaitUntilNSuccesses")
		})

		// termination signal
		safeWait <- struct{}{}
	})

	// to be executed once successes == n
	st.taskSuccesses.SetTriggerOnValue(n, triggerWaitUntilNSuccesses, func() {
		// kill all workers
		st.KillAllWorkers()

		// execute just after the trigger functions
		st.taskSuccesses.EnqueueToRunAfterCurrentTriggerFunctions(func() {
			// this function should be executed only once, so let's remove it after the first call
			st.taskSuccesses.UnsetTriggerOnValue(n, triggerWaitUntilNSuccesses)
		})

		// termination signal
		wait <- struct{}{}
	})

	// wait until n successes
	<-wait

	// now wait until totalWorkersInProgress == 0
	<-safeWait

	return nil
}

// WaitUntilInitialWorkersAreUp waits until all initial workers are up and running.
// The amount of initial workers were pre-defined at pool instantiation.
func (st *Pool) WaitUntilInitialWorkersAreUp() error {
	// verify that a previous call to this function is not still "in progress"
	if st.getStatus("WaitUntilInitialWorkersAreUp") {
		return newPoolError(ErrorWaitUntilInitialWorkersAreUpInProgress, "WaitUntilInitialWorkersAreUp is already in progress.")
	}

	st.setStatus("WaitUntilInitialWorkersAreUp", true)
	defer st.setStatus("WaitUntilInitialWorkersAreUp", false)

	<-st.waitUntilInitialWorkersAreUp

	return nil
}

// *************************************************************************************************************
// ** [Worker] Action's external functions  ********************************************************************
// *************************************************************************************************************

func (st *Pool) setupWorkerActionExternalFunctions(wkr *worker) {
	// worker's feedback after action/task processing
	wkr.SetExternalFeedbackFunction(func(wkrID int, reEnqueueWorker bool) {
		// sends the feedback info through a channel (the channel's listener resides in dispatcherWorkerFeedback)
		st.dispatcherWorkerFeedbackChan <- workerFeedback{
			workerID:        wkr.GetID(),
			reEnqueueWorker: reEnqueueWorker,
		}
	})

	// worker's fail func
	wkr.SetExternalFailFunc(st.workerFailFeedback)

	// kill worker
	wkr.SetExternalFunctionForAction(actionKillWorker, func() (reEnqueueWorker bool) {
		// There is no need to remove the worker here (st.removeWorker(wkr.GetID())), just return
		// false to let the dispatcherWorkerFeedback knows that this worker won't be re-enqueued.
		// Knowing that the dispatcherWorkerFeedback will remove the worker.

		// this worker could not be re-enqueued (it couldn't be re-used)
		reEnqueueWorker = false

		return
	})
}

// workerFailFeedback is executed once a worker's function exited because a panic. To the metrics, that scenario counts
// as a unsuccessful job.
func (st *Pool) workerFailFeedback(tsk *task, err error) {
	if err := st.panicTasks.Enqueue(tsk); err != nil {
		// TODO ::: include the task as part of the *PoolError, add a data interface{} field
		st.logError("Panicked task can't be enqueued into panicTasks", err)
	}
}

// *************************************************************************************************************
// ** Locks  ***************************************************************************************************
// *************************************************************************************************************

// isLockedForTasks returns whether the dispatcher is locked for tasks
func (st *Pool) isLockedForTasks() bool {
	return false
}

// lockForTasks locks dispatcher for locks, meaning that enqueued tasks will not be sent to workers
func (st *Pool) lockForTasks() {

}

// unlockForTasks unlocks dispatcher for locks, meaning enqueued tasks will be sent to workers
func (st *Pool) unlockForTasks() {

}

// *************************************************************************************************************
// ** Pause && Resume  *****************************************************************************************
// *************************************************************************************************************

// PauseAllWorkers pauses all workers.
// This action will wait until previous invoked actions get processed, but it will skip the line of enqueued jobs(tasks).
// The jobs that are being processed at the time this action is processed will not be interrupted.
// From the moment this action gets processed, all enqueued jobs/actions will not be processed until the workers get
// resumed by ResumeAllWorkers.
func (st *Pool) PauseAllWorkers() {
	// verify that a different PauseAllWorkers is not in progress
	if st.resumeWorkers == nil {
		st.resumeWorkers = make(chan struct{})

		// send an action to be executed by the dispatcher, not by a worker
		if err := st.addAction(actionPauseAllWorkers, false, func() {
			// wait until ResumeAllWorkers() gets invoked and send a signal over st.resumeWorkers
			<-st.resumeWorkers
			// close the channel and put it in nil
			close(st.resumeWorkers)
			st.resumeWorkers = nil
		}); err != nil {
			st.logError("PauseAllWorkers.addAction", err)
		}
	}
}

// ResumeAllWorkers resumes all workers.
// Nothing will happen if this function is invoked while no workers are paused.
func (st *Pool) ResumeAllWorkers() {
	if st.resumeWorkers != nil {
		st.resumeWorkers <- struct{}{}
	}
}

// *************************************************************************************************************
// ** External channels  ***************************************************************************************
// *************************************************************************************************************

// SetNewWorkerChan sets a channel to receive a signal once each new worker is started
func (st *Pool) SetNewWorkerChan(ch chan int) {
	st.externalNewWorkerChan = ch
}

// SetKilledWorkerChan sets a channel to receive a signal after a worker is terminated
func (st *Pool) SetKilledWorkerChan(ch chan int) {
	st.externalKilledWorkerChan = ch
}

// *************************************************************************************************************
// ** Metrics  *************************************************************************************************
// *************************************************************************************************************

// GetTotalWorkers returns the number of registered workers.
func (st *Pool) GetTotalWorkers() int {
	return st.totalWorkers.GetValue()
}

// GetTotalWorkersInProgress returns the amount of workers currently processing data (tasks/actions)
func (st *Pool) GetTotalWorkersInProgress() int {
	return st.totalWorkersInProgress.GetValue()
}

// GetPanicTasks returns the queue containing all tasks that failed because the worker's panicked while processing.
func (st *Pool) GetPanicTasks() goconcurrentqueue.Queue {
	return st.panicTasks
}

// *************************************************************************************************************
// ** Statuses. Operation in progress  *************************************************************************
// *************************************************************************************************************

// setStatus sets the status for a given operation
func (st *Pool) setStatus(operationName string, status bool) {
	st.generalInfoMap.Store(operationName, status)
}

// getStatus returns the status for a given operation
func (st *Pool) getStatus(operationName string) bool {
	rawValue, ok := st.generalInfoMap.Load(operationName)
	if !ok {
		return false
	}

	value := rawValue.(bool)
	return value
}
