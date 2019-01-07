// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.9.0  ********************************************************************************************************

// Package goworkerpool provides a pool of concurrent workers with the ability to increment / decrement / pause / resume workers on demand.
package goworkerpool

import (
	"log"
	"sync"

	"github.com/pkg/errors"
)

const (
	// immediate signal to kill a worker
	immediateSignalKillAfterTask = 0

	// error messages
	errorNoWorkerFuncMsg   = "The Worker Func is needed to invoke %v. You should set it using SetWorkerFunc(...)"
	errorKillAllWorkersMsg = "There is an active KillAllWorkers operation"
	errorNoStartWorkersMsg = "StartWorkers() needs to be invoked before this action"

	// "add new worker(s)" signal
	workerActionAdd = "add"
	// "kill worker(s)" signal
	workerActionKill = "kill"
	// confirm that a worker exited because a workerActionKill signal
	workerActionKillConfirmation = "kill.confirmation"

	// "kill all workers" signal
	workerActionKillAllWorkers = "killAllWorkers"
	// "kill all workers and wait" signal
	workerActionKillAllWorkersAndWait = "killAllWorkersAndWait"
	// confirm that a worker exited because a workerActionKillAllWorkers signal
	workerActionKillAllWorkersConfirmation = "killAllWorkers.confirmation"

	// "kill late worker" signal
	workerActionLateKill = "lateKill"
	// confirm that a worker exited because a workerActionKillConfirmation signal
	workerActionLateKillConfirmation = "lateKill.confirmation"
	// "late kill all worker" signal
	workerActionLateKillAllWorkers = "lateKillAllWorkers"
	// confirm that a worker received the LateKillAllWorkers signal, ran: KillAllWorkers and exited.
	workerActionLateKillAllWorkersConfirmation = "lateKillAllWorkers.confirmation"
	// confirm that a worker exited because an unhandled panic
	workerActionPanicKillConfirmation = "panicKill.confirmation"
	// confirm that a worker exited because the immediate channel is closed
	workerActionImmediateChanelClosedConfirmation = "immediateChannelClosed.confirmation"
	// SetTotalWorkers action
	workerActionSetTotalWorkers = "setTotalWorkers"

	// Wait()
	waitForWait = "wait"
	// WaitUntilNSuccesses()
	waitForNSuccesses = "waitNSuccesses"

	// broad messages
	broadMessagePause          = "pause"
	broadMessageKillAllWorkers = "killAllWorkers"

	// poolJobData codes
	poolJobDataCodeRegular            = "regular"
	poolJobDataCodeCallback           = "callback"
	poolJobDataCodeLateKillWorker     = "lateKillWorker"
	poolJobDataCodeLateKillAllWorkers = "lateKillAllWorkers"
)

// PoolFunc defines the function signature to be implemented by the worker's func
type PoolFunc func(interface{}) bool

// PoolCallback defines the callback function signature
type PoolCallback func(interface{})

// poolJobData contains the job data && internal pool data
type poolJobData struct {
	Code         string
	JobData      interface{}
	Category     string
	CallbackFunc PoolCallback
}

type Pool struct {
	// function to be executed by the workers
	fn PoolFunc
	// initial number of workers (at initialization moment)
	initialWorkers int
	// total live workers
	totalWorkers int
	// mutex to control totalWorkers access
	totalWorkersRWMutex sync.RWMutex
	// tells whether the initialWorkers were started
	workersStarted bool
	// tells the workers: do not accept / process new jobs
	doNotProcess bool

	// total needed job successes to finish WaitUntilNSuccesses(...)
	totalWaitUntilNSuccesses int

	// how many workers succeeded
	fnSuccessCounter int
	// how many workers failed
	fnFailCounter int
	// channel to send jobs, workers listen to this channel
	jobsChan chan poolJobData
	// channel to keep track of how many workers are up
	totalWorkersChan chan workerAction
	// channel to keep track of succeeded / failed jobs
	fnSuccessChan chan bool
	// channel to send "immediate" action's signals to workers
	immediateChan chan byte
	// channel to send signals once new workers are up
	newWorkerChan chan int
	// channel to send signals once a worker is killed
	killedWorkerChanel chan int

	// flag to know whether a Wait() function was called
	waitFor string
	// channel to wait the "done" signal for Wait()
	waitForWaitChannel chan bool
	// channel to send the "done" signal for WaitUntilNSuccesses(...)
	waitForNSuccessesChannel chan bool
	// channel to send the "done" signal after all workers get killed bc a workerActionKillAllWorkersAndWait signal
	waitForActionKillAllWorkersAndWait chan bool

	// to send the same message to all workers
	broadMessages sync.Map

	// log steps
	verbose bool
}

// workerAction have the data related to worker's actions:
//  - add new worker
//  - kill worker
//  - late kill worker
type workerAction struct {
	Action string
	Value  int
}

// NewPool creates, initializes and return a *Pool
func NewPool(initialWorkers int, maxJobsInQueue int, verbose bool) *Pool {
	ret := &Pool{}

	ret.initialize(initialWorkers, maxJobsInQueue, verbose)

	return ret
}

// pool internal initialization
func (st *Pool) initialize(initialWorkers int, maxJobsInQueue int, verbose bool) {
	st.jobsChan = make(chan poolJobData, maxJobsInQueue)
	st.totalWorkersChan = make(chan workerAction, 100)
	// the package will cause deadlock if st.fnSuccessChan is full
	st.fnSuccessChan = make(chan bool, maxJobsInQueue)

	// the workers were not started at this point
	st.workersStarted = false

	st.initialWorkers = initialWorkers

	st.verbose = verbose

	// GR to control the active workers successes / fails
	go st.fnSuccessListener()

	// worker's immediate action channel
	st.immediateChan = make(chan byte)

	st.waitForWaitChannel = make(chan bool)
	st.waitForNSuccessesChannel = make(chan bool)

	// GR to control the active workers counter / actions over workers
	go st.workerListener()

	// set broad messages default values
	st.broadMessages.Store(broadMessagePause, false)
	st.broadMessages.Store(broadMessageKillAllWorkers, false)
}

// SetNewWorkerChan sets a channel to receive signals after each new worker is started
func (st *Pool) SetNewWorkerChan(ch chan int) {
	st.newWorkerChan = ch
}

// SetKilledWorkerChan sets a channel to receive signals after worker(s) was/were killed
func (st *Pool) SetKilledWorkerChan(ch chan int) {
	st.killedWorkerChanel = ch
}

// sendKilledWorkerSignal sends an integer signal over a channel (the number of killed workers)
func (st *Pool) sendKilledWorkerSignal(value int) {
	if st.killedWorkerChanel != nil {
		st.killedWorkerChanel <- value
	}
}

// workerListener handles all up/down worker operations && keeps workers stats updated (st.totalWorkers)
func (st *Pool) workerListener() {
	keepListening := true
	for keepListening {

		// ************************************************************************************
		// ** Process first the following waitFor scenarios:  *********************************
		// **	- Wait()
		// ************************************************************************************
		switch st.waitFor {
		case waitForWait:
			if st.workersStarted && st.GetTotalWorkers() == 0 {
				// reset the st.waitFor variable to avoid processing again the "Wait ready" scenario
				st.waitFor = ""
				// send the signal to Wait() to let it know that no workers are alive
				st.waitForWaitChannel <- true
			}

		case waitForNSuccesses:
			// this case is handled by st.fnSuccessListener()

		default:
			// no waitFor signal
		}

		// ************************************************************************************
		// ** Process the actions over the workers  *******************************************
		// ************************************************************************************
		select {
		case message, ok := <-st.totalWorkersChan:
			// st.totalWorkersChan is closed
			if !ok {
				keepListening = false
				break
			}

			switch message.Action {
			// add new worker(s)
			case workerActionAdd:
				for i := 0; i < message.Value; i++ {
					// execute the worker function
					go st.workerFunc(st.totalWorkers)

					// critical section
					st.totalWorkersRWMutex.Lock()
					st.totalWorkers += 1
					st.totalWorkersRWMutex.Unlock()
					// critical section END

					// check whether all workers were started
					if !st.workersStarted && st.totalWorkers == st.initialWorkers {
						// the workers were started
						st.workersStarted = true
					}

					// send back a signal to let know that a new worker was started
					if st.newWorkerChan != nil {
						st.newWorkerChan <- 1
					}
				}

			// kill all workers
			case workerActionKillAllWorkers:
				// send a broad message to kill all workers
				st.broadMessages.Store(broadMessageKillAllWorkers, true)

			// kill all workers and wait until they get killed
			case workerActionKillAllWorkersAndWait:
				// send a broad message to kill all workers
				st.broadMessages.Store(broadMessageKillAllWorkers, true)

			// "kill all workers" confirmation from the worker
			case workerActionKillAllWorkersConfirmation:
				st.totalWorkers -= message.Value

				// set the "kill all workers" broad flag to false once all live workers were killed
				if st.totalWorkers == 0 {
					st.broadMessages.Store(broadMessageKillAllWorkers, false)

					// send the "done" signal to KillAllWorkersAndWait, all workers are down
					if st.waitForActionKillAllWorkersAndWait != nil {
						st.waitForActionKillAllWorkersAndWait <- true
					}
				}

				// send signal to let know that a worker was killed
				st.sendKilledWorkerSignal(message.Value)

			// kill worker(s)
			case workerActionKill:
				totalWorkers := st.GetTotalWorkers()
				if message.Value > totalWorkers {
					message.Value = totalWorkers
				}

				for i := 0; i < message.Value; i++ {
					st.immediateChan <- immediateSignalKillAfterTask
				}

			// "kill worker" confirmation from the worker
			// the worker was killed because a "immediate kill" signal
			case workerActionKillConfirmation:
				st.totalWorkers -= message.Value
				// send signal to let know that a worker was killed
				st.sendKilledWorkerSignal(message.Value)

			// late kill worker(s)
			case workerActionLateKill:
				totalWorkers := st.GetTotalWorkers()
				if message.Value > totalWorkers {
					// TODO ::: return error ???
					message.Value = totalWorkers
				}

				for i := 0; i < message.Value; i++ {
					st.jobsChan <- poolJobData{
						Code:    poolJobDataCodeLateKillWorker,
						JobData: nil,
					}
				}

			// "late kill worker" confirmation from the worker
			// the worker was killed because a "late kill" signal
			case workerActionLateKillConfirmation:
				st.totalWorkers -= message.Value
				// send signal to let know that a worker was killed
				st.sendKilledWorkerSignal(message.Value)

			// late kill all workers
			case workerActionLateKillAllWorkers:
				// enqueue a unique LateKillAllWorkers message, then the worker who catches it will kill all other workers
				st.jobsChan <- poolJobData{
					Code:    poolJobDataCodeLateKillAllWorkers,
					JobData: nil,
				}

			// "late kill all workers" was received and processed by a worker ("processed" ==> KillAllWorkers())
			case workerActionLateKillAllWorkersConfirmation:
				// TODO ::: stats

			// "immediate channel closed kill worker" confirmation from the worker
			// the worker was killed because the immediate channel is closed
			case workerActionImmediateChanelClosedConfirmation:
				st.totalWorkers -= message.Value
				// send signal to let know that a worker was killed
				st.sendKilledWorkerSignal(message.Value)

			// "panic kill worker" confirmation from the worker
			// the worker was killed because an unhandled panic
			case workerActionPanicKillConfirmation:
				st.totalWorkers -= message.Value
				// send signal to let know that a worker was killed
				st.sendKilledWorkerSignal(message.Value)

			// SetTotalWorkers(n)
			case workerActionSetTotalWorkers:
				currentTotalWorkers := st.GetTotalWorkers()

				// do nothing
				if message.Value < 0 || message.Value == currentTotalWorkers {
					continue
				}

				// kill some workers
				if message.Value < currentTotalWorkers {
					st.KillWorkers(currentTotalWorkers - message.Value)
					continue
				}

				// add extra workers
				st.AddWorkers(message.Value - currentTotalWorkers)
			}

		// no st.totalWorkersChan messages
		default:
		}
	}
}

// fnSuccessListener listens to the workers successes & fails
func (st *Pool) fnSuccessListener() {
	for fnSuccess := range st.fnSuccessChan {

		if fnSuccess {
			st.fnSuccessCounter++
			if st.verbose {
				log.Printf("[pool] fnSuccessCounter: %v workers: %v\n", st.fnSuccessCounter, st.totalWorkers)
			}

			if st.totalWaitUntilNSuccesses > 0 && st.fnSuccessCounter >= st.totalWaitUntilNSuccesses {
				st.waitForNSuccessesChannel <- true
			}
		} else {
			st.fnFailCounter++
		}
	}
}

// *************************************************************************************************************
// ** Wait functions  ******************************************************************************************
// *************************************************************************************************************

// Wait waits while at least one worker is up and running
func (st *Pool) Wait() error {
	if st.fn == nil {
		return errors.Errorf(errorNoWorkerFuncMsg, "Wait")
	}

	if st.GetTotalWorkers() == 0 {
		return nil
	}

	// set the waitFor flag for Wait()
	st.waitFor = waitForWait

	// wait here until all workers are done
	<-st.waitForWaitChannel

	if st.verbose {
		log.Println("[pool] No active workers. Wait() finished.")
	}

	return nil
}

// WaitUntilNSuccesses waits until n workers finished their job successfully, then kills all active workers.
// A worker is considered successfully if the associated worker function returned true.
// An error will be returned if the worker's function is not already set.
func (st *Pool) WaitUntilNSuccesses(n int) error {
	if st.fn == nil {
		return errors.Errorf(errorNoWorkerFuncMsg, "WaitUntilNSuccesses")
	}

	// set the number of jobs that have to be successfully processed
	st.totalWaitUntilNSuccesses = n

	// set the waitFor flag for Wait()
	st.waitFor = waitForNSuccesses

	// wait until n jobs get successfully processed
	<-st.waitForNSuccessesChannel

	// set the number of successful jobs to wait for to zero
	st.totalWaitUntilNSuccesses = 0

	// free the flag
	st.waitFor = ""

	// tell workers: do not accept / process new jobs && no new jobs can be accepted
	st.doNotProcess = true

	if st.verbose {
		log.Printf("[pool] WaitUntilNSuccesses: %v . kill all workers: %v\n", st.fnSuccessCounter, st.totalWorkers)
	}

	// kill all active workers
	st.KillAllWorkersAndWait()

	// tell workers: you can accept / process new jobs && start accepting new jobs
	st.doNotProcess = false

	return nil
}

// *************************************************************************************************************
// ** Set Handler Function  ************************************************************************************
// *************************************************************************************************************

// SetWorkerFunc sets the worker's function handler.
// This function will be invoked each time a worker pulls a new job, and should return true to let know that the job
// was successfully completed, or false in other case.
func (st *Pool) SetWorkerFunc(fn PoolFunc) {
	st.fn = fn
}

// *************************************************************************************************************
// ** Enqueue jobs  ********************************************************************************************
// *************************************************************************************************************

// AddTask will enqueue a job (into a FIFO queue: a channel).
//
// The parameter for the job's data accepts any type (interface{}).
//
// Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
// the job will stay in the queue until any worker will be ready to pick it up and start processing it.
//
// The queue in which this function enqueues the jobs has a limit (it was set up at pool initialization). It means that AddTask will wait
// for a free queue slot to enqueue a new job in case the queue is at full capacity.
//
// AddTask will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
// a certain amount of time when WaitUntilNSuccesses meets the stop condition.
func (st *Pool) AddTask(data interface{}) error {
	if !st.doNotProcess {
		// enqueue a regular job
		st.jobsChan <- poolJobData{
			Code:    poolJobDataCodeRegular,
			JobData: data,
		}
		return nil
	}

	return errors.New("No new jobs are accepted at this moment")
}

// AddTaskCallback enqueues a job plus a callback function into a FIFO queue (a channel).
//
// The parameter for the job's data accepts any type (interface{}).
//
// Workers (if alive) will be listening to and picking up jobs from this queue. If no workers are alive nor idle,
// the job will stay in the queue until any worker will be ready to pick it up and start processing it.
//
// The worker who picks up this job + callback will process the job first and later will invoke the callback function, passing the job's data as a parameter.
//
// The queue in which this function enqueues the jobs has a limit (it was set up at pool initialization). It means that AddTaskCallback will wait
// for a free queue slot to enqueue a new job in case the queue is at full capacity.
//
// AddTaskCallback will return an error if no new tasks could be enqueued at the execution time. No new tasks could be enqueued during
// a certain amount of time when WaitUntilNSuccesses meets the stop condition.
func (st *Pool) AddTaskCallback(data interface{}, callbackFn PoolCallback) error {
	if !st.doNotProcess {
		// enqueue a job + callback
		st.jobsChan <- poolJobData{
			Code:         poolJobDataCodeCallback,
			JobData:      data,
			CallbackFunc: callbackFn,
		}
		return nil
	}

	return errors.New("No new jobs are accepted at this moment")
}

// AddCallback enqueues a callback function into a FIFO queue (a channel).
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
	if !st.doNotProcess {
		// enqueue a callback-only job
		st.jobsChan <- poolJobData{
			Code:         poolJobDataCodeCallback,
			JobData:      nil,
			CallbackFunc: callbackFn,
		}
		return nil
	}

	return errors.New("No new jobs are accepted at this moment")
}

// *************************************************************************************************************
// ** Workers operations  **************************************************************************************
// *************************************************************************************************************

// StartWorkers starts all workers. The number of workers was set at the Pool instantiation (NewPool(...) function).
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) can be defined at SetNewWorkerChan(chan).
//
// StartWorkers will return an error if the worker function was not previously set.
func (st *Pool) StartWorkers() error {
	var err error
	for i := 0; i < st.initialWorkers; i++ {
		if err = st.startWorker(); err != nil {
			return err
		}
	}

	return nil
}

// StartWorkersAndWait starts all workers and waits until them are alive.
//
// This is a synchronous operation, but can be notified through a channel every time a new worker is started.
// In order to receive the signals on realtime, the listener should be running on a different goroutine.
//
// StartWorkersAndWait will return an error if the worker function was not previously set.
func (st *Pool) StartWorkersAndWait() error {
	sendSignalToClient := st.newWorkerChan != nil
	tmpClientChan := st.newWorkerChan

	// switch back to the client channel
	defer func() {
		if sendSignalToClient {
			st.SetNewWorkerChan(tmpClientChan)
		}
	}()

	// custom channel to receive signals for each started up worker
	tmpCustomChan := make(chan int, st.initialWorkers)
	// temporarily switch to our temp channel
	st.SetNewWorkerChan(tmpCustomChan)

	// start up the workers
	if err := st.StartWorkers(); err != nil {
		return err
	}

	totalWorkesUp := 0
	// start listening to "new worker" signals
	for message := range tmpCustomChan {
		totalWorkesUp += message

		// send "new worker" signals to client
		if sendSignalToClient {
			tmpClientChan <- message
		}

		// exit once the amount of workers == st.initialWorkers
		if totalWorkesUp == st.initialWorkers {
			break
		}
	}

	return nil
}

// startWorker starts a worker in a separate goroutine
// It will return an error if the worker function was not previously set.
func (st *Pool) startWorker() error {
	if st.fn == nil {
		return errors.Errorf(errorNoWorkerFuncMsg, "startWorker")
	}

	// increment the active workers by 1
	st.totalWorkersChan <- workerAction{
		Action: workerActionAdd,
		Value:  1,
	}

	return nil
}

// workerFunc keeps listening to st.jobsChan and executing st.fn(...)
func (st *Pool) workerFunc(n int) {
	// default kill worker confirmation
	killWorkerConfirmation := workerActionPanicKillConfirmation

	defer func() {
		// catch a panic that bubbled up
		if r := recover(); r != nil {
			// decrement the active workers by 1
			st.totalWorkersChan <- workerAction{
				Action: workerActionPanicKillConfirmation,
				Value:  1,
			}

			if st.verbose {
				log.Printf("[pool] worker %v is going to be down because of a panic", n)
			}
		}
	}()

	var (
		keepWorking            = true
		broadMsgTmp            interface{}
		broadMsgPause          bool
		broadMsgKillAllWorkers bool
		ok                     bool
	)

	for keepWorking {

		// *******************************************************************************************************
		// ** Listen to the immediate broad messages *************************************************************
		// ** These messages don't come through a channel, they come as part of st.broadMessages (sync.Map).  ****
		// ** The idea is to execute actions over the workers as soon as possible.  ******************************
		// *******************************************************************************************************
		// *** Actions:
		// ***  - Pause all workers
		// ***  - Kill all workers
		// *******************************************************************************************************

		// get the pause
		if broadMsgTmp, ok = st.broadMessages.Load(broadMessagePause); ok {
			broadMsgPause = broadMsgTmp.(bool)
		}

		// get the "kill all workers"
		if broadMsgTmp, ok = st.broadMessages.Load(broadMessageKillAllWorkers); ok {
			broadMsgKillAllWorkers = broadMsgTmp.(bool)

			if broadMsgKillAllWorkers {
				// confirm that the worker was killed due to a workerActionKill signal
				killWorkerConfirmation = workerActionKillAllWorkersConfirmation

				keepWorking = false
				break
			}
		}

		// *******************************************************************************************************
		// ** Listen to the immediate action channel *************************************************************
		// *******************************************************************************************************
		// *** Actions:
		// ***  - Kill
		// *******************************************************************************************************
		select {
		// listen to the immediate channel
		case immediate, ok := <-st.immediateChan:
			if !ok {
				if st.verbose {
					log.Printf("[pool] worker %v is going to be down because of the immediate channel is closed", n)
				}

				// confirm that the worker was killed due to the immediate channel is closed
				killWorkerConfirmation = workerActionImmediateChanelClosedConfirmation

				// break the loop
				keepWorking = false
				break
			}

			switch immediate {
			// kill the worker
			case immediateSignalKillAfterTask:
				// confirm that the worker was killed due to a workerActionKill signal
				killWorkerConfirmation = workerActionKillConfirmation

				keepWorking = false
				break
			}

		default:

		}

		// *******************************************************************************************************
		// ** Listen to the jobs channel *************************************************************************
		// *******************************************************************************************************
		// *** Actions:
		// ***  - LateKill
		// *******************************************************************************************************

		// do not listen to jobsChan if broadMessagePause == true
		if !broadMsgPause {
			select {
			// listen to the jobs/tasks channel
			case taskData, ok := <-st.jobsChan:
				if !ok {
					if st.verbose {
						log.Printf("[pool] worker %v is going to be down because of the jobs channel is closed", n)
					}
					// break the loop
					keepWorking = false
					break
				}

				switch taskData.Code {

				// regular job
				case poolJobDataCodeRegular:
					if st.doNotProcess {
						// TODO ::: re-enqueue in a different queue/channel/struct
						// re-enqueue the job / task
						st.AddTask(taskData.JobData)
					} else {
						// execute the job
						fnSuccess := st.fn(taskData.JobData)

						// avoid to cause deadlock
						if !st.doNotProcess {
							// keep track of the job's result
							st.fnSuccessChan <- fnSuccess
						} else {
							// TODO ::: save the job result ...
						}
					}

				// job + callback || callback
				case poolJobDataCodeCallback:
					if st.doNotProcess {
						st.AddTaskCallback(taskData.JobData, taskData.CallbackFunc)
					}

					// execute the job (in case there is some job data)
					if taskData.JobData != nil {
						// execute the job
						fnSuccess := st.fn(taskData.JobData)

						// avoid to cause deadlock
						if !st.doNotProcess {
							// keep track of the job's result
							st.fnSuccessChan <- fnSuccess
						} else {
							// TODO ::: save the job result ...
						}
					}

					// run the callback
					taskData.CallbackFunc(taskData.JobData)

				// late kill signal
				case poolJobDataCodeLateKillWorker:
					if st.verbose {
						log.Printf("[pool] worker %v is going to be down", n)
					}

					// confirm that the worker was killed due to a workerActionLateKill signal
					killWorkerConfirmation = workerActionLateKillConfirmation

					// break the loop
					keepWorking = false
					break

				// late kill all workers
				case poolJobDataCodeLateKillAllWorkers:
					if st.verbose {
						log.Printf("[pool] worker %v is going to be down :: LateKillAllWorkers()", n)
					}

					// confirm that the worker was killed due to a workerActionLateKill signal
					killWorkerConfirmation = workerActionLateKillConfirmation

					// kill all live workers
					st.KillAllWorkers()

					// confirm that the LateKillAllWorkers() was received executed
					st.totalWorkersChan <- workerAction{
						Action: workerActionLateKillAllWorkersConfirmation,
						Value:  1,
					}

					// break the loop
					keepWorking = false
					break

				default:

				}

			default:

			}
		}
	}

	// the worker is going to die, so decrement the active workers counter by 1
	//st.totalWorkersChan <- -1
	st.totalWorkersChan <- workerAction{
		Action: killWorkerConfirmation,
		Value:  1,
	}
}

// *************************************************************************************************************
// ** Workers adjustments && operations  ***********************************************************************
// *************************************************************************************************************

// SetTotalWorkers adjusts the number of live workers.
//
// In case it needs to kill some workers (in order to adjust the total based on the given parameter), it will wait until
// their current jobs get processed (in case they are processing jobs).
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) can be defined at SetNewWorkerChan(chan).
//
// SetTotalWorkers returns an error in the following scenarios:
//  - The workers were not started yet by StartWorkers.
//  - There is a "in course" KillAllWorkers operation.
func (st *Pool) SetTotalWorkers(n int) error {
	// verify that workers were started by StartWorkers()
	if !st.workersStarted {
		return errors.New(errorNoStartWorkersMsg)
	}

	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	// sends a "set total workers" signal, to be processed by workerListener()
	st.totalWorkersChan <- workerAction{
		Action: workerActionSetTotalWorkers,
		Value:  n,
	}

	return nil
}

// AddWorker adds a new worker to the pool.
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) can be defined at SetNewWorkerChan(chan).
//
// AddWorker returns an error if at least one of the following statements is true:
//  - the worker could not be started
//  - there is a "in course" KillAllWorkers operation
func (st *Pool) AddWorker() error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	return st.startWorker()
}

// AddWorkers adds n extra workers to the pool.
//
// This is an asynchronous operation, but you can be notified through a channel every time a new worker is started.
// The channel (optional) can be defined at SetNewWorkerChan(chan).
//
// AddWorkers returns an error if at least one of the following statements are true:
//  - the worker could not be started
//  - there is a "in course" KillAllWorkers operation
func (st *Pool) AddWorkers(n int) error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	var err error
	for i := 0; i < n; i++ {
		if err = st.AddWorker(); err != nil {
			return err
		}
	}

	return nil
}

// KillWorker kills an idle worker.
// The kill signal has a higher priority than the enqueued jobs. It means that a worker will be killed once it finishes its current job although there are unprocessed jobs in the queue.
// Use LateKillWorker() in case you need to wait until current enqueued jobs get processed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) KillWorker() error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	// sends a signal to kill a worker
	st.totalWorkersChan <- workerAction{
		Action: workerActionKill,
		Value:  1,
	}

	return nil
}

// KillWorkers kills n idle workers.
// If n > GetTotalWorkers(), then this function will assign GetTotalWorkers() to n.
// The kill signal has a higher priority than the enqueued jobs. It means that a worker will be killed once it finishes its current job, no matter if there are unprocessed jobs in the queue.
// Use LateKillAllWorkers() ot LateKillWorker() in case you need to wait until current enqueued jobs get processed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) KillWorkers(n int) error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	// sends a signal to kill n workers
	st.totalWorkersChan <- workerAction{
		Action: workerActionKill,
		Value:  n,
	}

	return nil
}

// KillAllWorkers kills all live workers (the number of live workers is determined at the moment this action is processed).
// If a worker is processing a job, it will not be immediately killed, the pool will wait until the current job gets processed.
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
func (st *Pool) KillAllWorkers() {
	// sends a signal to kill all active workers
	st.totalWorkersChan <- workerAction{
		Action: workerActionKillAllWorkers,
	}
}

// KillAllWorkersAndWait kills all live workers (the number of live workers is determined at the moment this action is processed).
// This function waits until current alive workers are down. This is the difference between KillAllWorkersAndWait() and KillAllWorkers()
// If a worker is processing a job, it will not be immediately killed, the pool will wait until the current job gets processed.
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
func (st *Pool) KillAllWorkersAndWait() {
	// critical section
	st.totalWorkersRWMutex.RLock()
	shouldReturn := st.totalWorkers == 0
	st.totalWorkersRWMutex.RUnlock()
	// critical section END

	// exit immediately if total workers == 0
	if shouldReturn {
		return
	}

	// the channel acts both as a channel and as a flag (as a flag to let know the pool that it has to send a signal through the channel once all workers are down)
	st.waitForActionKillAllWorkersAndWait = make(chan bool)

	// sends a signal to kill all active workers
	st.totalWorkersChan <- workerAction{
		Action: workerActionKillAllWorkersAndWait,
	}

	// wait for the done signal ==> all workers are down
	<-st.waitForActionKillAllWorkersAndWait

	st.waitForActionKillAllWorkersAndWait = nil
}

// LateKillWorker kills a worker only after current enqueued jobs get processed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) LateKillWorker() error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	// sends a signal to late kill a worker
	st.totalWorkersChan <- workerAction{
		Action: workerActionLateKill,
		Value:  1,
	}

	return nil
}

// LateKillWorkers kills n workers only after all current jobs get processed.
// If n > GetTotalWorkers(), then this function will assign GetTotalWorkers() to n.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) LateKillWorkers(n int) error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	// sends a signal to late kill n workers
	st.totalWorkersChan <- workerAction{
		Action: workerActionLateKill,
		Value:  n,
	}

	return nil
}

// LateKillAllWorkers kills all live workers only after all current jobs get processed.
// By "current jobs" it means: the number of enqueued jobs in the exact moment this function get executed.
// It returns an error in case there is a "in course" KillAllWorkers operation.
func (st *Pool) LateKillAllWorkers() error {
	// return en error if there is an "in course" KillAllWorkers operation
	if tmp, ok := st.broadMessages.Load(broadMessageKillAllWorkers); ok && tmp.(bool) {
		return errors.New(errorKillAllWorkersMsg)
	}

	st.totalWorkersChan <- workerAction{
		Action: workerActionLateKillAllWorkers,
	}

	return nil
}

// PauseAllWorkers pauses all live workers.
// The jobs that are being processed at the time this function is invoked will not be interrupted.
// All enqueued jobs will not be processed until the workers get resumed.
func (st *Pool) PauseAllWorkers() {
	st.broadMessages.Store(broadMessagePause, true)
}

// ResumeAllWorkers resumes all live workers.
func (st *Pool) ResumeAllWorkers() {
	st.broadMessages.Store(broadMessagePause, false)
}

// GetTotalWorkers returns the number of active/live workers.
func (st *Pool) GetTotalWorkers() int {
	st.totalWorkersRWMutex.RLock()
	defer st.totalWorkersRWMutex.RUnlock()

	return st.totalWorkers
}
