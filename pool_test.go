// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"sync"
	"testing"
	"time"

	"github.com/enriquebris/goconcurrentqueue"
	"github.com/stretchr/testify/suite"
)

type PoolTestSuite struct {
	suite.Suite
	pool *Pool
}

const (
	initialWorkers       = 5
	maxOperationsInQueue = 100
)

func (suite *PoolTestSuite) SetupTest() {
	var err error

	// build a *Pool and wait until all initial workers are up and running
	suite.pool, err = NewPoolWithOptions(PoolOptions{
		TotalInitialWorkers:          initialWorkers,
		MaxWorkers:                   10,
		MaxOperationsInQueue:         maxOperationsInQueue,
		WaitUntilInitialWorkersAreUp: true,
	})

	suite.NoError(err)
}

// ***************************************************************************************
// ** NewPool
// ***************************************************************************************

func (suite *PoolTestSuite) TestNewPoolWrongData() {
	pool := NewPool(-1, maxOperationsInQueue, false)
	suite.Nil(pool)

	pool = NewPool(initialWorkers, -1, false)
	suite.Nil(pool)
}

func (suite *PoolTestSuite) TestNewPool() {
	pool := NewPool(initialWorkers, maxOperationsInQueue, false)
	suite.NotNil(pool)
}

// ***************************************************************************************
// ** NewPoolWithOptions
// ***************************************************************************************

// new pool with MaxOperationsInQueue == 0 ==> error
func (suite *PoolTestSuite) TestNewPoolWithOptionsZeroMaxOperationsInQueue() {
	pool, err := NewPoolWithOptions(PoolOptions{
		TotalInitialWorkers:  5,
		MaxWorkers:           1,
		MaxOperationsInQueue: 0,
		NewWorkerChan:        nil,
	})

	suite.Nil(pool)
	suite.Error(err, "error expected if MaxOperationsInQueue == 0")
	// error's type
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equalf(ErrorData, pErr.Code(), "expected error's type: %v", ErrorData)
}

// new pool MaxWorkers == 0 ==> error
func (suite *PoolTestSuite) TestNewPoolWithOptionsZeroMaxWorkers() {
	pool, err := NewPoolWithOptions(PoolOptions{
		TotalInitialWorkers:  5,
		MaxWorkers:           0,
		MaxOperationsInQueue: 10,
		NewWorkerChan:        nil,
	})

	suite.Nil(pool)
	suite.Error(err, "error expected if MaxWorkers == 0")
	// error's type
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equalf(ErrorData, pErr.Code(), "expected error's type: %v", ErrorData)
}

// new pool with valid options (configuration parameters)
func (suite *PoolTestSuite) TestNewPoolWithOptions() {
	newWorkerChan := make(chan int, initialWorkers+1)

	pool, err := NewPoolWithOptions(PoolOptions{
		TotalInitialWorkers:  initialWorkers,
		MaxWorkers:           10,
		MaxOperationsInQueue: maxOperationsInQueue,
		NewWorkerChan:        newWorkerChan,
	})

	suite.NoError(err)

	// wait until initial workers are up
	totalWorkers := 0
	for totalWorkers < initialWorkers {
		select {
		case newWorker := <-newWorkerChan:
			totalWorkers = totalWorkers + newWorker
		case <-time.After(5 * time.Second):
			suite.FailNow("Too much time waiting for a worker initialization")
		}
	}

	// check values defined at initialization
	suite.checkInitializationValues(pool)
}

// new pool && wait until all initial workers are up and running
func (suite *PoolTestSuite) TestNewPoolWithOptionsWaitUntilInitialWorkersAreUp() {
	var (
		pool     *Pool
		err      error
		poolIsUp = make(chan struct{})
	)

	go func() {
		pool, err = NewPoolWithOptions(PoolOptions{
			TotalInitialWorkers:          initialWorkers,
			MaxWorkers:                   10,
			MaxOperationsInQueue:         maxOperationsInQueue,
			NewWorkerChan:                nil,
			WaitUntilInitialWorkersAreUp: true,
		})

		suite.NoError(err)
		poolIsUp <- struct{}{}
	}()

	select {
	case <-poolIsUp:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for workers initialization")
	}

	// combined with this WaitUntilInitialWorkersAreUp should be the deprecated function StartWorkersAndWait, the
	// whole following section could be removed once StartWorkersAndWait gets removed
	startWorkersAndWaitIsDone := make(chan struct{})
	go func() {
		err = pool.StartWorkersAndWait()
		suite.NoError(err)

		startWorkersAndWaitIsDone <- struct{}{}
	}()

	select {
	case <-startWorkersAndWaitIsDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for workers StartWorkersAndWait")
	}

	// check values defined at initialization
	suite.checkInitializationValues(pool)
}

// ***************************************************************************************
// ** initialization
// ***************************************************************************************

// checkInitializationValues checks values defined in initialization()
func (suite *PoolTestSuite) checkInitializationValues(pool *Pool) {
	// max operations in queue
	suite.Equal(maxOperationsInQueue, cap(pool.activityChan))

	// workers
	// verify that suite.pool.totalWorkers == initialWorkers
	suite.Equal(initialWorkers, pool.totalWorkers.GetValue())
	// verify that all workers are available
	suite.Equal(initialWorkers, pool.availableWorkers.GetLen())
	// no workers in progress
	suite.Equal(0, pool.totalWorkersInProgress.GetValue(), "unexpected workers in progress just after initialization")

	// no actions / tasks
	suite.Zero(pool.actions.GetLen(), "unexpected enqueued actions just after initialization")
	suite.Zero(pool.tasks.GetLen(), "unexpected enqueued tasks just after initialization")

	// metrics
	suite.Zero(pool.taskSuccesses.GetValue())

	// no actions in progress
	suite.False(pool.getStatus(waitInProgress), "unexpected Wait action in progress")
	suite.False(pool.getStatus(killAllWorkersInProgress), "unexpected KillAllWorkers in progress")

	// trigger on totalWorkers.0.triggerZeroTotalWorkers
	suite.NotNil(pool.totalWorkers.GetTriggerOnValue(0, triggerZeroTotalWorkers), "trigger function expected for totalWorkers.0.triggerZeroTotalWorkers")

	// verify that all workers have different IDs
	suite.checkWorkersID(pool)
}

// initialize() with AddWorker() failing because KillAllWorkers is in progress
func (suite *PoolTestSuite) TestInitializationFailAddWorker() {
	// mimic KillAllWorkers in progress to avoid create new workers
	suite.pool.setStatus(killAllWorkersInProgress, true)

	// set log channel
	logChan := make(chan PoolLog, initialWorkers)
	suite.pool.SetLogChan(logChan)

	doneChan := make(chan []PoolLog)
	// listen to log channel
	go func(logChan chan PoolLog, max int, doneChan chan []PoolLog) {
		total := 0
		poolLogList := make([]PoolLog, 0)
		for total < max-1 {
			poolLog := <-logChan
			poolLogList = append(poolLogList, poolLog)
			total++
		}

		doneChan <- poolLogList
	}(logChan, initialWorkers, doneChan)

	// initialize the pool
	suite.pool.initialize(PoolOptions{
		TotalInitialWorkers:          initialWorkers,
		MaxWorkers:                   10,
		MaxOperationsInQueue:         maxOperationsInQueue,
		WaitUntilInitialWorkersAreUp: true,
	})

	// get total workers && availableWorkers
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	select {
	case logs := <-doneChan:
		// check the error logs
		for i := 0; i < len(logs); i++ {
			// log's code: error
			suite.Equal(logError, logs[i].Code)
			// error's type
			pErr, ok := logs[i].Error.(*PoolError)
			suite.True(ok)
			suite.Equal(ErrorKillAllWorkersInProgress, pErr.Code())
		}
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	// check that no workers were added to workers / availableWorkers
	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())
}

// point pool.totalWorkers to zero to trigger the totalWorkers.zero trigger's function
func (suite *PoolTestSuite) TestInitializationTriggerTotalWorkersZero() {
	suite.Equal(initialWorkers, suite.pool.totalWorkers.GetValue())

	// mimic KillAllWorkers in progress
	suite.pool.setStatus(killAllWorkersInProgress, true)
	// mimic Wait in progress
	suite.pool.setStatus(waitInProgress, true)
	// set noWorkersChan (part of Wait "in progress")
	noWorkersChan := make(chan struct{}, 2)
	suite.pool.noWorkersChan = noWorkersChan

	// update totalWorkers to zero
	for suite.pool.totalWorkers.GetValue() >= 0 {
		suite.pool.totalWorkers.Update(-1)
	}

	select {
	case <-suite.pool.getNoWorkersChan():
		// "no workers" signal received
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for noWorkers channel")
	}

	// killAllWorkersInProgress should be updated to false
	suite.False(suite.pool.getStatus(killAllWorkersInProgress))
}

// ***************************************************************************************
// ** dispatcher - action
// ***************************************************************************************

// send a signal over activityChan while no action/task was enqueued
func (suite *PoolTestSuite) TestDispatcherNoActionNoTask() {
	suite.True(suite.pool.getStatus(dispatcherInProgress))
	defer suite.True(suite.pool.getStatus(dispatcherInProgress))

	// set log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// mimic "send action/task"
	select {
	case suite.pool.activityChan <- struct{}{}:
	default:
		suite.FailNow("can't send message over activityChannel")
	}

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		pErr, ok := pLog.Error.(*PoolError)
		suite.True(ok, "expected error's type: *PoolError")
		suite.Equal(ErrorDispatcherNoActionNoTask, pErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}
}

// dispatcher stops after activityChann is closed
func (suite *PoolTestSuite) TestDispatcherClosedActivityChan() {
	close(suite.pool.activityChan)
	// give the dispatcher 1second to update dispatcherAlive status
	time.Sleep(time.Second)
	suite.False(suite.pool.getStatus(dispatcherInProgress))
}

// dispatcher receives an "activity" signal but actions.Dequeue() fails
func (suite *PoolTestSuite) TestDispatcherActionQueueError() {
	// replace action's queue
	suite.pool.actions = goconcurrentqueue.NewFixedFIFO(0)
	// lock the action's queue, any future queue's operation will fail
	suite.pool.actions.Lock()

	// set log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// mimic "send action/task"
	select {
	case suite.pool.activityChan <- struct{}{}:
	default:
		suite.FailNow("can't send message over activityChannel")
	}

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
		suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
		suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	suite.True(suite.pool.getStatus(dispatcherInProgress))
}

// dispatcher receives an action to be executed only by itself, not by the worker
func (suite *PoolTestSuite) TestDispatcherActionDoNotSentToWorker() {
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	// to let know that the preFunc() was executed
	preFuncDone := make(chan struct{}, 2)

	// enqueue the action
	err := suite.pool.addAction("dummyAction", false, func() {
		preFuncDone <- struct{}{}
	})
	suite.NoError(err)

	// wait until action's preFunc execution is done
	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for action's preFunc execution")
	}

	notSupposedToHappenChan := make(chan struct{}, 2)
	// add a trigger to totalWorkers - 1 ==> fail and wait some short time !!!
	suite.pool.totalWorkers.SetTriggerOnValue(totalWorkers-1, "error", func(currentValue int, previousValue int) {
		notSupposedToHappenChan <- struct{}{}
	})

	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())

	select {
	case <-notSupposedToHappenChan:
		suite.FailNow("available workers' amount should not change after dispatcher executes an action with sendToWorker == false")
	case <-time.After(3 * time.Second):
		// available workers' amount keep the same after 3 seconds ==> it's ok, it's the expected, no any worker
		// processed the enqueued action
	}

	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())
}

// dispatcher receives an action to be executed by a worker, but can't dequeue a worker and later can't re-enqueue the
// failed action
func (suite *PoolTestSuite) TestDispatcherActionByWorkerCantDequeueWorker() {
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalActions := suite.pool.actions.GetLen()
	totalTasks := suite.pool.tasks.GetLen()

	// to let know that the preFunc() was executed
	preFuncDone := make(chan struct{}, 2)
	// to receive pool's logs
	logChan := make(chan PoolLog, 2)
	// set log channel
	suite.pool.SetLogChan(logChan)

	// replace availableWorkers internal queue
	suite.pool.availableWorkers = goconcurrentqueue.NewFixedFIFO(1)
	// lock the queue, so Dequeue operation will fail
	suite.pool.availableWorkers.Lock()

	// enqueue the action
	err := suite.pool.addAction("dummyAction", true, func() {
		// lock internal's pool actions queue, all future operations over it will fail
		suite.pool.actions.Lock()

		preFuncDone <- struct{}{}
	})
	suite.NoError(err)

	// wait until action's preFunc execution is done
	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for action's preFunc execution")
	}

	// wait for the first && second error:
	//  1 - no worker couldn't be dequeued from availableWorkers queue
	//	2 - failed action couldn't be enqueued into actions queue
	for i := 0; i < 2; i++ {
		select {
		case pLog := <-logChan:
			suite.Equal(logError, pLog.Code)
			qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
			suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
			suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
		case <-time.After(5 * time.Second):
			suite.FailNow("Too much time waiting for log channel messages")
		}
	}

	// same amount of workers expected
	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	// same amount of actions as before the action was enqueued
	suite.Equal(totalActions, suite.pool.actions.GetLen())
	// same amount of tasks as before the action was enqueued
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
}

// dispatcher receives an action to be executed by a worker, but can't dequeue a worker and later action  will be
// re-enqueued
func (suite *PoolTestSuite) TestDispatcherActionByWorkerCantDequeueWorkerReEnqueueAction() {
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalActions := suite.pool.actions.GetLen()
	totalTasks := suite.pool.tasks.GetLen()

	// to let know that the preFunc() was executed
	preFuncDone := make(chan struct{}, 2)
	// to receive pool's logs
	logChan := make(chan PoolLog, 2)
	// set log channel
	suite.pool.SetLogChan(logChan)

	// replace availableWorkers internal queue
	suite.pool.availableWorkers = goconcurrentqueue.NewFixedFIFO(1)
	// lock the queue, so Dequeue operation will fail
	suite.pool.availableWorkers.Lock()

	// enqueue the action
	err := suite.pool.addAction("dummyAction", true, func() {
		preFuncDone <- struct{}{}
	})
	suite.NoError(err)

	// wait until action's preFunc execution is done
	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for action's preFunc execution")
	}

	// wait for the logChannel: [error] no worker couldn't be dequeued from availableWorkers queue
	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
		suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
		suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	// dispatcher failed to dequeue a worker to process the action, so it re-enqueued the action
	// wait until action's preFunc execution is done
	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for action's preFunc execution")
	}

	// same amount of workers
	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	// same amount of actions as before the action was enqueued
	suite.Equal(totalActions, suite.pool.actions.GetLen())
	// same amount of tasks as before the action was enqueued
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
}

// dispatcher receives an action to be executed by a worker
func (suite *PoolTestSuite) TestDispatcherActionByWorker() {
	// build a *Pool (only 1 worker) and wait until all initial workers are up and running
	var err error
	suite.pool, err = NewPoolWithOptions(PoolOptions{
		TotalInitialWorkers:          1,
		MaxWorkers:                   10,
		MaxOperationsInQueue:         maxOperationsInQueue,
		WaitUntilInitialWorkersAreUp: true,
	})
	suite.NoError(err)

	// metrics
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()
	totalActions := suite.pool.actions.GetLen()
	totalTasks := suite.pool.tasks.GetLen()
	taskSuccesses := suite.pool.taskSuccesses.GetValue()

	// channel to know when the postActionFunc is done
	externalPostActionFuncDone := make(chan struct{}, 2)
	// add a ExternalPostActionFunc to worker to know when the action was processed by a worker
	// dequeue the only worker
	rawWorker, err := suite.pool.availableWorkers.Dequeue()
	suite.NoError(err)
	wkr, ok := rawWorker.(*worker)
	suite.True(ok, "expected worker's type: *worker")
	wkr.SetExternalPostActionFunc(func() {
		externalPostActionFuncDone <- struct{}{}
	})
	// re-enqueue the worker into availableWorkers
	err = suite.pool.availableWorkers.Enqueue(wkr)
	suite.NoError(err, "unexpected error while enqueueing worker into availableWorkers")

	// to let know that the preFunc() was executed
	preFuncDone := make(chan struct{}, 2)

	// enqueue the action
	err = suite.pool.addAction("dummyAction", true, func() {
		preFuncDone <- struct{}{}
	})
	suite.NoError(err)

	// wait until action's preFunc execution is done
	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for action's preFunc execution")
	}

	select {
	case <-externalPostActionFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for worker's postActionFunc")
	}

	// same amount of workers
	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	// same amount of actions as before the action was enqueued
	suite.Equal(totalActions, suite.pool.actions.GetLen())
	// same amount of tasks as before the action was enqueued
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
	// same amount of task successes
	suite.Equal(taskSuccesses, suite.pool.taskSuccesses.GetValue())
	// same amount of available workers
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())
}

// ***************************************************************************************
// ** dispatcher - regular task
// ***************************************************************************************

// dispatcher receives an "activity signal" but tasks' dequeue fails
func (suite *PoolTestSuite) TestDispatcherTaskCantDequeueTask() {
	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// lock the tasks queue, any future dequeue attempt will fail
	suite.pool.tasks.Lock()

	// send a signal over activityChannel
	select {
	case suite.pool.activityChan <- struct{}{}:
	default:
		suite.FailNow("can't send signals over activityChan")
	}

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
		suite.True(ok, "expected error's type: *goconcurrent.QueueError")
		suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}
}

// dispatcher receives a task but can't dequeue a worker to pass the task
func (suite *PoolTestSuite) TestDispatcherTaskCantDequeueWorker() {
	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// lock availableWorkers to avoid dequeue any worker
	suite.pool.availableWorkers.Lock()

	suite.NoError(suite.pool.AddTask("dummy task"))

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
		suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
		suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}
}

// dispatcher receives a task to be executed by a worker
func (suite *PoolTestSuite) TestDispatcherTask() {
	workerDone := make(chan interface{}, 2)
	// set default worker's handler
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		workerDone <- data
		return true
	})
	taskData := "dummy data"
	suite.NoError(suite.pool.AddTask(taskData))

	select {
	case data := <-workerDone:
		// verify the task's data
		suite.Equal(taskData, data)
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for worker's function execution")
	}
}

// ***************************************************************************************
// ** dispatcher - late action (tasks)
// ***************************************************************************************

// dispatcher receives a late action (task) but the data is not a proper action
func (suite *PoolTestSuite) TestDispatcherTaskLateActionWrongType() {
	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// send the late action
	suite.NoError(suite.pool.addTask(taskLateAction, "incorrect data type", nil, nil))

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}
}

// dispatcher receives a late action (task) + pre-task function to be executed by the dispatcher
func (suite *PoolTestSuite) TestDispatcherTaskLateActionDoNotSendToWorker() {
	// metrics
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()
	totalActions := suite.pool.actions.GetLen()
	totalTasks := suite.pool.tasks.GetLen()
	taskSuccesses := suite.pool.taskSuccesses.GetValue()

	preFuncDone := make(chan struct{}, 2)

	// send the late action
	suite.NoError(suite.pool.addTask(taskLateAction,
		action{
			Code:         actionLateKillWorker,
			SendToWorker: false,
			PreExternalFunc: func() {
				preFuncDone <- struct{}{}
			},
		},
		nil,
		nil))

	select {
	case <-preFuncDone:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for preLateTask function")
	}

	// same amount of workers
	suite.Equal(totalWorkers, getSyncMapTotalLen(suite.pool.workers))
	// same amount of actions as before the action was enqueued
	suite.Equal(totalActions, suite.pool.actions.GetLen())
	// same amount of tasks as before the action was enqueued
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
	// same amount of task successes
	suite.Equal(taskSuccesses, suite.pool.taskSuccesses.GetValue())
	// same amount of available workers
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())
}

// ***************************************************************************************
// ** dispatcher worker feedback
// ***************************************************************************************

// dispatcherWorkerFeedback receives a message for a unknown worker
func (suite *PoolTestSuite) TestDispatcherWorkerFeedbackUnknownWorker() {
	totalWorkers := suite.pool.GetTotalWorkers()
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	// replace the external feedback function for all workers
	suite.pool.workers.Range(func(key interface{}, value interface{}) bool {
		wkr, _ := value.(*worker)

		wkr.SetExternalFeedbackFunction(func(workerID int, reEnqueueWorker bool) {
			// remove the worker's entry, so the external feedback function will fail
			suite.pool.workers.Delete(workerID)

			// sends the feedback info through a channel (the channel's listener resides in dispatcherWorkerFeedback)
			suite.pool.dispatcherWorkerFeedbackChan <- workerFeedback{
				workerID:        workerID,
				reEnqueueWorker: reEnqueueWorker,
			}
		})

		return true
	})

	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// set worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// send dummy task
	suite.pool.AddTask("dummy data")

	// wait for log channel
	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		suite.Equal("dispatcherWorkerFeedback.unknown worker", pLog.Message)
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	// verify that no extra workers were enqueued
	suite.Equal(totalWorkers, suite.pool.GetTotalWorkers())
	suite.Equal(totalAvailableWorkers-1, suite.pool.availableWorkers.GetLen())
}

// dispatcherWorkerFeedback receives a message to re-enqueue a worker but the attempt fails
func (suite *PoolTestSuite) TestDispatcherWorkerFeedbackCantReEnqueueWorker() {
	totalWorkers := suite.pool.GetTotalWorkers()
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// set worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		// lock the availableWorkers queue will prevent any near-future attempt to enqueue workers
		suite.pool.availableWorkers.Lock()
		return true
	})

	// send dummy task
	suite.pool.AddTask("dummy data")

	// wait for log channel
	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)

		suite.Equal("dispatcherWorkerFeedback.availableWorkers.Enqueue", pLog.Message)
		qErr, ok := pLog.Error.(*goconcurrentqueue.QueueError)
		suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
		suite.Equal(goconcurrentqueue.QueueErrorCodeLockedQueue, qErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	// verify that 1 worker was removed from the lists
	suite.Equal(totalWorkers-1, suite.pool.GetTotalWorkers())
	suite.Equal(totalAvailableWorkers-1, suite.pool.availableWorkers.GetLen())
}

// dispatcherWorkerFeedback receives a message to not re-enqueue a worker but the removing attempt fails
func (suite *PoolTestSuite) TestDispatcherWorkerFeedbackCantRemoveWorker() {
	totalWorkers := getSyncMapTotalLen(suite.pool.workers)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	// log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	// send the signal to dispatcherWorkerFeedback
	go func() {
		// add a fake worker (wrong worker's type)
		suite.pool.workers.Store(-1, "wrong.worker.type")

		select {
		case suite.pool.dispatcherWorkerFeedbackChan <- workerFeedback{
			workerID:        -1,
			reEnqueueWorker: false,
		}:
		default:
			suite.FailNow("couldn't send signal to dispatcherWorkerFeedback")
		}
	}()

	select {
	case pLog := <-logChan:
		suite.Equal(logError, pLog.Code)
		pErr, ok := pLog.Error.(*PoolError)
		suite.True(ok, "expected error's type: *PoolError")
		suite.Equal(ErrorWorkerType, pErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log channel messages")
	}

	// verify metrics
	// I added a fake worker to workers
	suite.Equal(totalWorkers+1, getSyncMapTotalLen(suite.pool.workers))
	suite.Equal(totalAvailableWorkers, suite.pool.availableWorkers.GetLen())
}

// dispatcherWorkerFeedback receives a signal to not re-enqueue the worker, so it will be removed
func (suite *PoolTestSuite) TestDispatcherWorkerFeedbackRemoveWorker() {

}

// dispatcherWorkerFeedback receives a signal to re-Enqueue the worker
func (suite *PoolTestSuite) TestDispatcherWorkerFeedback() {

}

// ***************************************************************************************
// ** workers
// ***************************************************************************************

// countTotalWorkers returns the number of workers from a given pool
func countTotalWorkers(pool *Pool) int {
	totalWorkers := 0
	pool.workers.Range(func(key interface{}, value interface{}) bool {
		totalWorkers++
		return true
	})

	return totalWorkers
}

// checkWorkersID checks that every worker has a different ID
func (suite *PoolTestSuite) checkWorkersID(pool *Pool) {
	suite.Equal(pool.totalWorkers.GetValue(), countTotalWorkers(pool), "duplicated worker's ID")
}

// ***************************************************************************************
// ** AddWorker
// ***************************************************************************************

// AddWorker() with KillAllWorkers in progress
func (suite *PoolTestSuite) TestAddWorkerKillAllWorkersInProgress() {
	suite.pool.setStatus(killAllWorkersInProgress, true)

	err := suite.pool.AddWorker()
	suite.Error(err, "Error expected if KillAllWorkers is in progress")

	pErr, ok := err.(*PoolError)
	suite.True(ok, "Expected error's type: *PoolError")
	suite.Equal(ErrorKillAllWorkersInProgress, pErr.Code())
}

// AddWorker()
func (suite *PoolTestSuite) TestAddWorker() {
	totalWorkers := countTotalWorkers(suite.pool)
	totalAvailableWorkers := suite.pool.availableWorkers.GetLen()

	// get current workers' IDs
	workersIDs := getSyncMapKeys(suite.pool.workers)

	// new worker channel
	newWorkerChan := make(chan int, 2)
	suite.pool.SetNewWorkerChan(newWorkerChan)

	err := suite.pool.AddWorker()
	suite.NoError(err)

	// wait until the "new worker" notification arrives
	select {
	case <-newWorkerChan:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for new worker")
	}

	// totalWorkers by workers map
	suite.Equal(totalWorkers+1, countTotalWorkers(suite.pool), "same amount of workers as before AddWorker() was executed")
	// totalWorkers
	suite.Equal(totalWorkers+1, suite.pool.totalWorkers.GetValue())
	// new worker into availableWorkers queue
	suite.Equal(totalAvailableWorkers+1, suite.pool.availableWorkers.GetLen())

	// get new worker
	newWorkers := getSyncMapNewItems(suite.pool.workers, workersIDs)
	// expected: only 1 new worker
	suite.Equal(1, len(newWorkers), "only 1 new worker expected")
	// check worker's type: *worker
	newWorker, ok := newWorkers[0].(*worker)
	suite.True(ok)
	// check new worker's configuration
	suite.checkWorkerConfiguration(newWorker)
}

// checkWorkerConfiguration checks the worker's configuration (based on *Pool.setupWorker)
func (suite *PoolTestSuite) checkWorkerConfiguration(wkr *worker) {
	// external task successes
	suite.NotNil(suite.pool.taskSuccesses, wkr.externalTaskSuccesses)

	totalWorkersInProgress := suite.pool.GetTotalWorkersInProgress()

	// worker's external preTask function
	workerExternalPreTaskFunction := wkr.externalPreTaskFunc
	suite.NotNil(workerExternalPreTaskFunction)
	workerExternalPreTaskFunction()
	suite.Equal(totalWorkersInProgress+1, suite.pool.GetTotalWorkersInProgress())

	// update totalWorkerInProgress
	totalWorkersInProgress = suite.pool.GetTotalWorkersInProgress()

	// worker's external postTask function
	workerExternalPostTaskFunction := wkr.externalPostTaskFunc
	suite.NotNil(workerExternalPostTaskFunction)
	workerExternalPostTaskFunction()
	suite.Equal(totalWorkersInProgress-1, suite.pool.GetTotalWorkersInProgress())

	// update totalWorkerInProgress
	totalWorkersInProgress = suite.pool.GetTotalWorkersInProgress()

	// worker's external preAction function
	workerExternalPreActionFunction := wkr.externalPreActionFunc
	suite.NotNil(workerExternalPreActionFunction)
	workerExternalPreActionFunction()
	suite.Equal(totalWorkersInProgress+1, suite.pool.GetTotalWorkersInProgress())

	// update totalWorkerInProgress
	totalWorkersInProgress = suite.pool.GetTotalWorkersInProgress()

	// worker's external postAction function
	workerExternalPostActionFunction := wkr.externalPostActionFunc
	suite.NotNil(workerExternalPostActionFunction)
	workerExternalPostActionFunction()
	suite.Equal(totalWorkersInProgress-1, suite.pool.GetTotalWorkersInProgress())
}

// AddWorker() sending a newWorker signal over a channel without listener: it can't send the signal
func (suite *PoolTestSuite) TestAddWorkerFullNewWorkerChannel() {
	// new worker channel
	newWorkerChan := make(chan int)
	suite.pool.SetNewWorkerChan(newWorkerChan)

	// set log channel
	logChan := make(chan PoolLog, 2)
	suite.pool.SetLogChan(logChan)

	err := suite.pool.AddWorker()
	suite.NoError(err)

	select {
	case logData := <-logChan:
		suite.Equal(logError, logData.Code)
		pErr, ok := logData.Error.(*PoolError)
		// custom error: *PoolError
		suite.True(ok, "expected error's type: *PoolError")
		// custom error's code
		suite.Equal(ErrorFullCapacityChannel, pErr.Code())
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for log's channel")
	}
}

// AddWorker() but can't enqueue the new worker into availableWorkers queue
func (suite *PoolTestSuite) TestAddWorkerCantEnqueueInAvailableWorkers() {
	// replace the availableWorkers pool's queue by an empty && 0-capacity queue
	suite.pool.availableWorkers = goconcurrentqueue.NewFixedFIFO(0)

	err := suite.pool.AddWorker()
	suite.Error(err)
}

// ***************************************************************************************
// ** AddWorkers
// ***************************************************************************************

// AddWorkers() with n <= 0
func (suite *PoolTestSuite) TestAddWorkersZeroWorkers() {
	totalWorkers := suite.pool.GetTotalWorkers()

	// adding 0 workers
	err := suite.pool.AddWorkers(0)
	suite.Error(err)
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equal(ErrorData, pErr.Code())
	// same amount of workers as before the function call
	suite.Equal(totalWorkers, suite.pool.GetTotalWorkers())

	// adding -1 workers
	err = suite.pool.AddWorkers(-1)
	suite.Error(err)
	pErr, ok = err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equal(ErrorData, pErr.Code())
	// same amount of workers as before the function call
	suite.Equal(totalWorkers, suite.pool.GetTotalWorkers())
}

// AddWorkers() with KillAllWorkers in progress
func (suite *PoolTestSuite) TestAddWorkersKillAllWorkersInProgress() {
	totalWorkers := suite.pool.GetTotalWorkers()

	// mimic KillAllWorkers in progress
	suite.pool.setStatus(killAllWorkersInProgress, true)

	err := suite.pool.AddWorkers(5)
	suite.Error(err)
	// check custom error type
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equal(ErrorEmbeddedErrors, pErr.Code())
	// check embedded errors
	suite.Equal(5, len(pErr.GetEmbeddedErrors()))
	// check each embedded error
	for i := 0; i < len(pErr.GetEmbeddedErrors()); i++ {
		pEmbErr, okE := pErr.GetEmbeddedErrors()[i].(*PoolError)
		suite.True(okE, "expected error's type: *PoolError")
		suite.Equal(ErrorKillAllWorkersInProgress, pEmbErr.Code())
	}

	// same amount of workers as before the function call
	suite.Equal(totalWorkers, suite.pool.GetTotalWorkers())
}

// AddWorkers()
func (suite *PoolTestSuite) TestAddWorkers() {
	totalWorkers := suite.pool.GetTotalWorkers()
	// get current workers' IDs
	workersIDs := getSyncMapKeys(suite.pool.workers)

	newWorkerChan := make(chan int, 2)
	done := make(chan struct{}, 2)
	go func(ch chan int, done chan struct{}, total int) {
		c := 0
		for {
			<-ch
			c++

			if c == total-1 {
				done <- struct{}{}
			}
		}
	}(newWorkerChan, done, 5)

	suite.pool.SetNewWorkerChan(newWorkerChan)
	suite.NoError(suite.pool.AddWorkers(5))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for new workers")
	}

	// check amount of workers
	suite.Equal(totalWorkers+5, suite.pool.GetTotalWorkers())
	suite.Equal(totalWorkers+5, suite.pool.availableWorkers.GetLen())

	// check new workers' configuration
	newWorkers := getSyncMapNewItems(suite.pool.workers, workersIDs)
	suite.Equal(totalWorkers+5, totalWorkers+len(newWorkers))
	for i := 0; i < len(newWorkers); i++ {
		wkr, ok := newWorkers[i].(*worker)
		suite.True(ok, "expected type for the new worker: *worker")
		suite.checkWorkerConfiguration(wkr)
	}
}

// ***************************************************************************************
// ** AddTask
// ***************************************************************************************

// AddTask() while KillAllWorkers in progress
func (suite *PoolTestSuite) TestAddTaskKillAllWorkersInProgress() {
	suite.pool.setStatus(killAllWorkersInProgress, true)

	// total tasks before call AddTask()
	totalTasks := suite.pool.tasks.GetLen()

	err := suite.pool.AddTask("any data")
	suite.Error(err, "expected error after calling pool.AddTask while KillAllWorkers in progress")
	// custom error type && code
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected custom error's type: *PoolError")
	suite.Equal(ErrorKillAllWorkersInProgress, pErr.Code())

	// total tasks must be the same as before calling AddTask
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
}

// AddTask() with no default workers' function handler
func (suite *PoolTestSuite) TestAddTaskNoDefaultWorkerFunc() {
	// total tasks before call AddTask()
	totalTasks := suite.pool.tasks.GetLen()

	err := suite.pool.AddTask("any data")
	suite.Error(err, "expected error after calling pool.AddTask with no default worker's function handler")
	// custom error type && code
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected custom error's type: *PoolError")
	suite.Equal(ErrorNoDefaultWorkerFunction, pErr.Code())

	// total tasks must be the same as before calling AddTask
	suite.Equal(totalTasks, suite.pool.tasks.GetLen())
}

// AddTask() that could not enqueue the task
func (suite *PoolTestSuite) TestAddTaskCouldNotEnqueueTask() {
	// set default worker's handler function
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// replace tasks queue by a zero-capacity queue
	suite.pool.tasks = goconcurrentqueue.NewFixedFIFO(0)

	err := suite.pool.AddTask("dummy task")
	suite.Error(err, "expected error after task could not be enqueued into the tasks' queue")
	qErr, ok := err.(*goconcurrentqueue.QueueError)
	suite.True(ok, "expected error's type: *goconcurrentqueue.QueueError")
	suite.Equal(goconcurrentqueue.QueueErrorCodeFullCapacity, qErr.Code())
}

// AddTask() could not send a signal over activityChan after enqueues the new task
func (suite *PoolTestSuite) TestAddTaskFullCapacityActivityChan() {
	// set default worker's handler function
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// replace the activityChan by a zero-capacity channel, so no data could be enqueued
	suite.pool.activityChan = make(chan struct{}, 0)

	err := suite.pool.AddTask("dummy data")
	suite.Error(err)
	// check custom error
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equal(ErrorDispatcherChannelFull, pErr.Code())
	// task.Valid should be false
	rawTsk, err := suite.pool.tasks.Dequeue()
	suite.NoError(err, "unexpected error")
	tsk, ok := rawTsk.(*task)
	suite.True(ok)
	suite.False(tsk.Valid, "task.Valid should be false")
}

// AddTask()
func (suite *PoolTestSuite) TestAddTask() {
	totalTasks := suite.pool.tasks.GetLen()
	suite.Equal(0, totalTasks, "unexpected enqueued task")

	doneWorkerDefaultFunc := make(chan struct{}, 2)
	// set default worker's handler function
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		select {
		case doneWorkerDefaultFunc <- struct{}{}:
		default:
			suite.FailNow("Could not send 'done' signal over doneWorkerDefaultFunc")
		}

		return true
	})

	// pause workers
	suite.pool.PauseAllWorkers()

	taskData := "dummy data"
	err := suite.pool.AddTask(taskData)
	suite.NoError(err)

	// as all workers are paused the task was enqueued but not taken by a worker, so the tasks' queue was
	// incremented by 1
	suite.Equal(totalTasks+1, suite.pool.tasks.GetLen(), "")

	// verify the enqueued task
	rawTsk, _ := suite.pool.tasks.Dequeue()
	tsk, ok := rawTsk.(*task)
	suite.True(ok, "expected task wrapper type: *task")
	// task data
	tskData, ok := tsk.Data.(string)
	suite.True(ok, "expected task's data type: string")
	suite.Equal(taskData, tskData)
	// task type
	suite.Equal(taskRegular, tsk.Code)
	// task valid
	suite.True(tsk.Valid)
	// task callback
	suite.Nil(tsk.CallbackFunc)
	// task handler function
	taskHandlerFunc := tsk.Func
	taskHandlerFunc(tsk.Data)
	// wait for the task's handler function "done" signal
	select {
	case <-doneWorkerDefaultFunc:
	default:
		suite.FailNow("expected task's function was not executed")
	}
}

// ***************************************************************************************
// ** addAction
// ***************************************************************************************

// addAction() that could not enqueue the action
func (suite *PoolTestSuite) TestAddActionCouldNotEnqueueAction() {
	// replace actions' queue by a zero-capacity queue
	suite.pool.actions = goconcurrentqueue.NewFixedFIFO(0)

	err := suite.pool.addAction("dummyCode", true, func() {})
	suite.Error(err)
	_, ok := err.(*goconcurrentqueue.QueueError)
	suite.True(ok)
}

// addAction() could not send a signal over activityChan after enqueues the new task
func (suite *PoolTestSuite) TestAddActionFullCapacityActivityChan() {
	// replace the activityChan by a zero-capacity channel, so no data could be enqueued
	suite.pool.activityChan = make(chan struct{}, 0)

	err := suite.pool.addAction("dummyCode", true, func() {})
	suite.Error(err)
	// custom error
	pErr, ok := err.(*PoolError)
	suite.True(ok)
	suite.Equal(ErrorDispatcherChannelFull, pErr.Code())
}

// ***************************************************************************************
// ** Metrics: GetTotalWorkers, GetTotalWorkersInProgress
// ***************************************************************************************

func (suite *PoolTestSuite) TestGetTotalWorkers() {
	suite.Equal(suite.pool.totalWorkers.GetValue(), suite.pool.GetTotalWorkers())
}

func (suite *PoolTestSuite) TestGetTotalWorkersInProgress() {
	suite.Equal(suite.pool.totalWorkersInProgress.GetValue(), suite.pool.GetTotalWorkersInProgress())
}

// ***************************************************************************************
// ** Misc
// ***************************************************************************************

// getSyncMapTotal returns mp's length
func getSyncMapTotalLen(mp sync.Map) int {
	total := 0

	mp.Range(func(key interface{}, value interface{}) bool {
		total++
		return true
	})

	return total
}

// getSyncMapKeys returns map's keys
func getSyncMapKeys(mp sync.Map) []interface{} {
	keys := make([]interface{}, 0)
	mp.Range(func(key interface{}, value interface{}) bool {
		keys = append(keys, key)
		return true
	})

	return keys
}

// getSyncMapNewItems returns the new items on newMap. It compares a key's slice from a previous state
// to the current keys.
func getSyncMapNewItems(newMap sync.Map, oldMapKeys []interface{}) []interface{} {
	ret := make([]interface{}, 0)
	tmpKeyMap := make(map[interface{}]struct{})
	for i := 0; i < len(oldMapKeys); i++ {
		tmpKeyMap[oldMapKeys[i]] = struct{}{}
	}

	newMap.Range(func(key interface{}, value interface{}) bool {
		if _, ok := tmpKeyMap[key]; !ok {
			ret = append(ret, value)
		}
		return true
	})

	return ret
}

// ***************************************************************************************
// ** Run suite
// ***************************************************************************************

func TestPoolErrorTestSuite(t *testing.T) {
	suite.Run(t, new(PoolTestSuite))
}
