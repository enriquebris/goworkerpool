// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"testing"
	"time"

	"github.com/enriquebris/goconcurrentcounter"
	"github.com/stretchr/testify/suite"
)

const (
	workerID = 100
)

type WorkerTestSuite struct {
	suite.Suite
	wkr *worker
}

func (suite *WorkerTestSuite) SetupTest() {
	suite.wkr = newWorker(workerID)
}

// ***************************************************************************************
// ** Initialization
// ***************************************************************************************

// initialization values
func (suite *WorkerTestSuite) TestInitialization() {
	// GetID
	suite.Equal(workerID, suite.wkr.GetID())

	// channels should be empty and not closed
	// tasks channel
	select {
	case <-suite.wkr.GetTasksChannel():
		suite.FailNow("tasks channel should be empty and not closed just after initialization")
	default:
	}

	// actions channel
	select {
	case <-suite.wkr.GetActionsChannel():
		suite.FailNow("actions channel should be empty and not closed just after initialization")
	default:
	}

	// nil ExternalTaskSuccesses at initialization
	suite.Nil(suite.wkr.GetExternalTaskSuccesses())

	// no external function for actions
	suite.Equal(0, len(suite.wkr.externalActions))

	// no external function for action
	suite.Equal(0, len(suite.wkr.externalActions))

	// external feedback function
	suite.Nil(suite.wkr.externalFeedbackFunc)

	// external preAction func
	suite.Nil(suite.wkr.externalPreActionFunc)

	// external postAction func
	suite.Nil(suite.wkr.externalPostActionFunc)

	// external preTask func
	suite.Nil(suite.wkr.externalPreTaskFunc)

	// external postTask func
	suite.Nil(suite.wkr.externalPostTaskFunc)

	// error channel
	suite.Nil(suite.wkr.GetErrorChannel())

	// metrics
	// zero tasks successes
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	// zero tasks failures
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// closed
	suite.False(suite.wkr.getStatus(WorkerClosed))
}

// verifies: setupInternalFunctionOperations adds internal functions for actionKillWorker && actionLateKillWorker and
// both functions return false
func (suite *WorkerTestSuite) TestSetupInternalFunctionOperations() {
	// 2 internal function for actions
	suite.Equal(2, len(suite.wkr.internalActions))

	// actionKillWorker
	f1, ok1 := suite.wkr.internalActions[actionKillWorker]
	suite.True(ok1)
	// f1 returns false
	suite.False(f1())

	// actionLateKillWorker
	f2, ok2 := suite.wkr.internalActions[actionLateKillWorker]
	suite.True(ok2)
	// f2 returns false
	suite.False(f2())

}

// ***************************************************************************************
// ** GetTasksChannel && GetActionsChannel
// ***************************************************************************************

func (suite *WorkerTestSuite) TestGetTasksChannel() {
	suite.NotNil(suite.wkr.GetTasksChannel(), "task channel should not be nil")
}

func (suite *WorkerTestSuite) TestGetActionsChannel() {
	suite.NotNil(suite.wkr.GetActionsChannel(), "actions channel should not be nil")
}

// ***************************************************************************************
// ** ErrorChannel
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetErrorChannel() {
	var errorChan chan *PoolError
	suite.wkr.SetErrorChannel(errorChan)

	suite.Equal(errorChan, suite.wkr.GetErrorChannel())
}

// ***************************************************************************************
// ** Close
// ***************************************************************************************

// Close() while Listen not running
func (suite *WorkerTestSuite) TestCloseWithListenNotInProgress() {
	suite.False(suite.wkr.getStatus(ListenInProgress))
	// close all channels
	suite.wkr.Close()

	suite.checkClosedChannels()
	suite.False(suite.wkr.getStatus(ListenInProgress))
}

// Close() while Listen is running
func (suite *WorkerTestSuite) TestCloseWithListenInProgress() {
	suite.wkr.Listen()

	listenIsUp := make(chan struct{})
	// send a dummy task to be executed by the worker, once the task is processed we
	// know the Listen is up
	suite.NoError(suite.wkr.ProcessTask(&task{
		Code: taskRegular,
		Data: nil,
		Func: nil,
		CallbackFunc: func(data interface{}) {
			listenIsUp <- struct{}{}
		},
		Valid: true,
	}))

	// wait until the task is processed by Listen, meaning Listen is up
	select {
	case <-listenIsUp:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for task processing")
	}
	// close the worker
	listenerDoneChan := suite.wkr.Close()
	// all channels should be closed
	suite.checkClosedChannels()

	// wait until Listen is not longer listening
	<-listenerDoneChan

	// Listen is not running
	suite.False(suite.wkr.getStatus(ListenInProgress))
	suite.True(suite.wkr.Closed())
}

// checkClosedChannels verifies that channels are closed
func (suite *WorkerTestSuite) checkClosedChannels() {
	// close channel
	select {
	case _, ok := <-suite.wkr.closeChan:
		suite.False(ok, "close channel should be closed")
	default:
		suite.FailNow("unexpected error")
	}

	// tasks channel
	select {
	case _, ok := <-suite.wkr.GetTasksChannel():
		suite.False(ok, "tasks channel should be closed")
	default:
		suite.FailNow("unexpected error")
	}

	// actions channel
	select {
	case _, ok := <-suite.wkr.GetActionsChannel():
		suite.False(ok, "actions channel should be closed")
	default:
		suite.FailNow("unexpected error")
	}
}

// ***************************************************************************************
// ** SetExternalTaskSuccesses && GetExternalTaskSuccesses
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalTaskSuccesses() {
	cint := goconcurrentcounter.NewIntMutex(0)
	suite.wkr.SetExternalTaskSuccesses(cint)

	suite.Equal(cint, suite.wkr.GetExternalTaskSuccesses(), "wrong external task successes")
}

// ***************************************************************************************
// ** SetExternalFunctionForAction
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalFunctionForAction() {
	dummyFunc := func() (reEnqueueWorker bool) {
		reEnqueueWorker = true
		return
	}

	suite.wkr.SetExternalFunctionForAction("action1", dummyFunc)
	f, ok := suite.wkr.externalActions["action1"]
	suite.True(ok, "an entry should exists for 'action1'")
	// functions return
	suite.Equal(dummyFunc(), f())
}

// ***************************************************************************************
// ** SetExternalFeedbackFunction
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalFeedbackFunction() {
	dummyIntValue := 0
	dummyFunc := func(workerID int, reEnqueueWorker bool) {
		dummyIntValue = workerID
	}

	suite.wkr.SetExternalFeedbackFunction(dummyFunc)
	fn := suite.wkr.externalFeedbackFunc
	suite.NotNil(fn, "external feedback func should not be nil")
	// execute function
	fn(100, true)
	suite.Equal(100, dummyIntValue)
}

// ***************************************************************************************
// ** SetExternalPreActionFunc
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalPreActionFunc() {
	dummyIntValue := 0
	dummyFunc := func() {
		dummyIntValue = 50
	}

	suite.wkr.SetExternalPreActionFunc(dummyFunc)
	fn := suite.wkr.externalPreActionFunc

	suite.NotNil(fn, "external preAction func should not be nil")
	// execute function
	fn()
	suite.Equal(50, dummyIntValue)
}

// ***************************************************************************************
// ** SetExternalPostActionFunc
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalPostActionFunc() {
	dummyIntValue := 0
	dummyFunc := func() {
		dummyIntValue = 50
	}

	suite.wkr.SetExternalPostActionFunc(dummyFunc)
	fn := suite.wkr.externalPostActionFunc

	suite.NotNil(fn, "external postAction func should not be nil")
	// execute function
	fn()
	suite.Equal(50, dummyIntValue)
}

// ***************************************************************************************
// ** SetExternalPreTaskFunc
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalPreTaskFunc() {
	dummyIntValue := 0
	dummyFunc := func() {
		dummyIntValue = 50
	}

	suite.wkr.SetExternalPreTaskFunc(dummyFunc)
	fn := suite.wkr.externalPreTaskFunc

	suite.NotNil(fn, "external preAction func should not be nil")
	// execute function
	fn()
	suite.Equal(50, dummyIntValue)
}

// ***************************************************************************************
// ** SetExternalPostTaskFunc
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetExternalPostTaskFunc() {
	dummyIntValue := 0
	dummyFunc := func() {
		dummyIntValue = 50
	}

	suite.wkr.SetExternalPostTaskFunc(dummyFunc)
	fn := suite.wkr.externalPostTaskFunc

	suite.NotNil(fn, "external preAction func should not be nil")
	// execute function
	fn()
	suite.Equal(50, dummyIntValue)
}

// ***************************************************************************************
// ** ProcessTask && ProcessAction
// ***************************************************************************************

func (suite *WorkerTestSuite) TestProcessTaskNilTask() {
	err := suite.wkr.ProcessTask(nil)

	suite.Error(err, "error expected for nil task")
	pErr, ok := err.(*PoolError)
	suite.True(ok, "expected error's type: *PoolError")
	suite.Equal(ErrorWorkerNilTask, pErr.Code())
}

func (suite *WorkerTestSuite) TestProcessTaskRegularTaskNoFunc() {
	err := suite.wkr.ProcessTask(&task{
		Code: taskRegular,
		Func: nil,
	})

	suite.Error(err, "error expected if regularTask comes without a function to process the data")

	if pErr, ok := err.(*PoolError); !ok {
		suite.FailNow("expected error's type: *PoolError")
	} else {
		suite.Equalf(ErrorWorkerNoTaskFunc, pErr.Code(), "expected error's code: %v", taskRegular)
	}

}

func (suite *WorkerTestSuite) TestProcessTask() {
	cntinue := make(chan struct{})
	cntinue2 := make(chan struct{})
	dummyTask := &task{}

	// fake Listen function
	go func() {
		cntinue <- struct{}{}
		tsk := <-suite.wkr.GetTasksChannel()

		suite.Equal(dummyTask, tsk, "received task is not what was sent")

		cntinue2 <- struct{}{}
	}()

	// wait here until the fake Listen ^ is up and running
	<-cntinue

	err := suite.wkr.ProcessTask(dummyTask)

	// wait here until the fake Listen function is done
	<-cntinue2

	suite.Equal(0, len(suite.wkr.GetTasksChannel()))
	suite.NoError(err)
}

func (suite *WorkerTestSuite) TestProcessAction() {
	cntinue := make(chan struct{})
	cntinue2 := make(chan struct{})
	dummyAction := action{}

	// fake Listen function
	go func() {
		cntinue <- struct{}{}
		tsk := <-suite.wkr.GetActionsChannel()

		suite.Equal(dummyAction, tsk, "received action is not what was sent")

		cntinue2 <- struct{}{}
	}()

	// wait here until the fake Listen ^ is up and running
	<-cntinue

	suite.wkr.ProcessAction(dummyAction)

	suite.Equal(0, len(suite.wkr.GetActionsChannel()))

	// wait here until the fake Listen function is done
	<-cntinue2
}

// ***************************************************************************************
// ** Listen
// ***************************************************************************************

type externalFeedbackFunctionData struct {
	workerID        int
	reEnqueueWorker bool
}

// Process three different actions in a row:
//	1st action: no external functions, no internal function, worker keep listen for new data
//	2nd action: external actions, no internal function, worker keep listen for new data
//	3rd action: external functions, internal function returning false ==> worker won't be available to process new data
func (suite *WorkerTestSuite) TestListenAction() {
	externalFeedbackFunctionChan := make(chan externalFeedbackFunctionData, 2)
	// set external feedback function
	suite.wkr.SetExternalFeedbackFunction(func(workerID int, reEnqueueWorker bool) {
		externalFeedbackFunctionChan <- externalFeedbackFunctionData{workerID, reEnqueueWorker}
	})

	// start listening
	suite.wkr.Listen()

	// *************************************************************************
	// first action with no external functions to execute, no internal function
	// *************************************************************************

	// first action without external functions to execute, nothing will happen
	suite.wkr.ProcessAction(action{
		SendToWorker:    true,
		Code:            "dummyActionCode",
		PreExternalFunc: nil,
	})

	// wait here until the first dummy action gets processed and the listener gets ready for something new
	var feedbackData externalFeedbackFunctionData
	select {
	case feedbackData = <-externalFeedbackFunctionChan:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for an action")
	}
	suite.Equal(suite.wkr.GetID(), feedbackData.workerID)
	suite.True(feedbackData.reEnqueueWorker, "worker should be ready to process new data")

	// *************************************************************************
	// second action with external functions to execute, no internal function
	// *************************************************************************
	tmpValue := 0
	dummyAction := action{
		SendToWorker: true,
		Code:         "dummyAction",
	}
	// set external preAction (dummy function updating tmpValue to 100, so it would be easy to verify whether this function was executed)
	suite.wkr.SetExternalPreActionFunc(func() { tmpValue = 100 })

	// channel to wait until external post action's function gets executed
	cntinuePostAction := make(chan struct{}, 2)
	// set external post action function
	suite.wkr.SetExternalPostActionFunc(func() { cntinuePostAction <- struct{}{} })

	// channel to wait until external action's function gets executed
	waitForExternalFunc2ndAction := make(chan struct{}, 2)
	// set external action function
	suite.wkr.SetExternalFunctionForAction(dummyAction.Code, func() (reEnqueueWorker bool) {
		defer func() {
			waitForExternalFunc2ndAction <- struct{}{}
		}()
		return true
	})

	// send the action to be processed
	suite.wkr.ProcessAction(dummyAction)

	// wait here until second action's external function get executed
	select {
	case <-waitForExternalFunc2ndAction:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for an action's external function")
	}

	// verify external preAction (for 2nd action)
	suite.Equal(tmpValue, 100)

	// wait here until feedback is received (for 2nd action)
	select {
	case feedbackData = <-externalFeedbackFunctionChan:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for an action")
	}
	suite.Equal(suite.wkr.GetID(), feedbackData.workerID)
	suite.True(feedbackData.reEnqueueWorker, "worker should be ready to process new data")

	// wait here until external post action's function get executed
	select {
	case <-cntinuePostAction:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for a post action's external function")
	}

	// *************************************************************************
	// third action with external functions to execute and internal function
	//  returning false
	// *************************************************************************

	// reset tmpValue
	tmpValue = 0

	waitForActionKillWorkerExternalFunction := make(chan struct{}, 2)
	// set external function for third action
	suite.wkr.SetExternalFunctionForAction(actionKillWorker, func() (reEnqueueWorker bool) {
		defer func() {
			waitForActionKillWorkerExternalFunction <- struct{}{}
		}()

		reEnqueueWorker = true
		return
	})

	// send third action
	suite.wkr.ProcessAction(action{
		Code:         actionKillWorker,
		SendToWorker: true,
	})

	// wait here until third action external's function gets executed
	<-waitForActionKillWorkerExternalFunction

	// verify external pre action function
	suite.Equal(100, tmpValue, "external pre action func should be executed before this point")

	// wait here for the external feedback
	select {
	case feedbackData = <-externalFeedbackFunctionChan:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for an action's external function")
	}
	// verify feedback data
	suite.Equal(suite.wkr.GetID(), feedbackData.workerID)
	suite.False(feedbackData.reEnqueueWorker, "reEnqueueWorker should be false because the internal function returned false")

	// verify external post action
	// wait here until external post action's function get executed
	select {
	case <-cntinuePostAction:
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for a post action's external function")
	}
}

// Process four different regular tasks in a row:
//	1 - task (data + function), no callback, no external functions
//	2 - task (data + function) + callback, no external functions
//	3 - task (data + function returning false, meaning => fail), no callback, no external functions
//	4 - task (data + function) + callback + external functions
func (suite *WorkerTestSuite) TestListenRegularTask() {
	// start listening
	suite.wkr.Listen()

	// set external task successes counter
	externalTaskSuccesses := goconcurrentcounter.NewIntMutex(0)
	suite.wkr.SetExternalTaskSuccesses(externalTaskSuccesses)

	// *************************************************************************
	// first task: data + function, no callback, no external functions
	// *************************************************************************

	// to wait until 1st task's function gets executed
	waitForTaskFunction := make(chan struct{}, 2)

	// 1st task
	tsk := &task{
		Code: taskRegular,
		Data: 100,
		Func: func(data interface{}) bool {
			defer func() {
				waitForTaskFunction <- struct{}{}
			}()
			return true
		},
	}

	// send the task
	err := suite.wkr.ProcessTask(tsk)
	suite.NoError(err, "no error expected for task + function + no callback + no external functions")

	// wait for the task's function
	select {
	case <-waitForTaskFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for task's function execution")
	}

	// verify the metrics, the task's function returned true so externalTaskSuccesses should reflect it
	suite.Equal(1, externalTaskSuccesses.GetValue())
	suite.Equal(1, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// *************************************************************************
	// second task: data + function + callback, no external functions
	// *************************************************************************

	// to wait until 2nd task's function gets executed
	waitForTask2Function := make(chan struct{}, 2)
	// to wait until 2nd task's callback gets executed
	waitFor2ndTaskCallback := make(chan struct{}, 2)

	// 2nd task
	tsk2 := &task{
		Code: taskRegular,
		Data: 150,
		Func: func(data interface{}) bool {
			defer func() {
				waitForTask2Function <- struct{}{}
			}()
			return true
		},
		CallbackFunc: func(data interface{}) {
			waitFor2ndTaskCallback <- struct{}{}
		},
	}

	// send the task
	err = suite.wkr.ProcessTask(tsk2)
	suite.NoError(err, "no error expected for task + function + callback + no external functions")

	// wait until 2nd task's function gets executed
	select {
	case <-waitForTask2Function:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd task's function execution")
	}

	// wait until 2nd task's callback gets executed
	select {
	case <-waitFor2ndTaskCallback:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd task's callback execution")
	}

	// verify 2nd success
	suite.Equal(2, externalTaskSuccesses.GetValue())
	suite.Equal(2, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// *************************************************************************
	// third task: data + function (returning false), no callback, no external
	// functions
	// *************************************************************************

	// to wait until 3rd task's function gets executed
	waitForTask3Function := make(chan struct{}, 2)

	// 3rd task
	tsk3 := &task{
		Code: taskRegular,
		Data: 200,
		Func: func(data interface{}) bool {
			defer func() {
				waitForTask3Function <- struct{}{}
			}()
			return false
		},
	}

	// send task
	err = suite.wkr.ProcessTask(tsk3)
	suite.NoError(err, "no error expected for task + function (returning false) + no callback + no external functions")

	select {
	case <-waitForTask3Function:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd task's function execution")
	}

	// verify 3rd fail
	suite.Equal(2, externalTaskSuccesses.GetValue())
	suite.Equal(2, suite.wkr.GetTaskSuccesses())
	// verify that failCounter == 1
	suite.Equal(1, suite.wkr.GetTaskFailures())

	// *************************************************************************
	// fourth task: data + function (returning true) + callback + external
	// functions
	// *************************************************************************

	// to wait until 4th task's function gets executed
	waitForTask4Function := make(chan struct{}, 2)
	// to wait until 4th task's callback gets executed
	waitFor4thTaskCallback := make(chan struct{}, 2)
	// to wait until pre-task function gets executed
	waitForPreTaskFunction := make(chan struct{}, 2)
	// to wait until post-task function gets executed
	waitForPostTaskFunction := make(chan struct{}, 2)

	// set external functions
	// pre-task function
	suite.wkr.SetExternalPreTaskFunc(func() {
		waitForPreTaskFunction <- struct{}{}
	})
	// post-task function
	suite.wkr.SetExternalPostTaskFunc(func() {
		waitForPostTaskFunction <- struct{}{}
	})

	// 4th task
	tsk4 := &task{
		Code: taskRegular,
		Data: 250,
		Func: func(data interface{}) bool {
			defer func() {
				waitForTask4Function <- struct{}{}
			}()
			return true
		},
		CallbackFunc: func(data interface{}) {
			waitFor4thTaskCallback <- struct{}{}
		},
	}

	// send 4th task
	suite.wkr.ProcessTask(tsk4)

	// wait until pre-task function
	select {
	case <-waitForPreTaskFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 4th task's pre-function execution")
	}

	// wait until 4th task's function
	select {
	case <-waitForTask4Function:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 4th task's function execution")
	}

	// wait until 4th task's callback
	select {
	case <-waitFor4thTaskCallback:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 4th task's callback execution")
	}

	// metrics
	suite.Equal(3, externalTaskSuccesses.GetValue())
	suite.Equal(3, suite.wkr.GetTaskSuccesses())
	suite.Equal(1, suite.wkr.GetTaskFailures())

	// wait until 4th post-task function
	select {
	case <-waitForPostTaskFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 4th task's post-task function execution")
	}
}

// Process four different late actions in a row:
//	1 - late task action + internal function + external feedback function, no external function
//	2 - late task action + external functions + internal function + callback
//	3 - late task action + external function + internal function returning false + callback
func (suite *WorkerTestSuite) TestListenLateActionTask() {
	// start listening
	suite.wkr.Listen()

	// *************************************************************************
	// 1st task: late action + internal function, no external function
	// *************************************************************************

	waitFor1stInternalFunction := make(chan struct{}, 2)
	waitForExternalFeedbackFunction := make(chan externalFeedbackFunctionData, 2)

	// set external feedback function
	suite.wkr.SetExternalFeedbackFunction(func(workerId int, reEnqueueWorker bool) {
		waitForExternalFeedbackFunction <- externalFeedbackFunctionData{workerID, reEnqueueWorker}
	})

	// set internal function
	suite.wkr.setInternalFunctionForAction([]string{"taskLateDummy1"}, func() bool {
		defer func() {
			waitFor1stInternalFunction <- struct{}{}
		}()

		return true
	})

	tsk1 := &task{
		Code: taskLateAction,
		Data: action{
			Code: "taskLateDummy1",
		},
	}

	// process task1 (late-action)
	suite.wkr.ProcessTask(tsk1)

	// wait until 1st action's internal function gets executed
	select {
	case <-waitFor1stInternalFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 1st action's internal function function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait until external feedback function gets executed for 1st action
	select {
	case feedback := <-waitForExternalFeedbackFunction:
		suite.Equal(suite.wkr.GetID(), feedback.workerID)
		suite.True(feedback.reEnqueueWorker)
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 1st action's feedback function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// *************************************************************************
	// 2nd task: late task action + external functions, no internal function
	// *************************************************************************

	tsk2 := &task{
		Code: taskLateAction,
		Data: action{
			Code: "taskLateDummy2",
		},
	}

	waitForExternalPreActionFunction := make(chan struct{}, 2)
	waitFor2ndActionInternalFunction := make(chan struct{}, 2)
	waitForExternalFunction2ndAction := make(chan struct{}, 2)
	waitForExternalPostActionFunction := make(chan struct{}, 2)

	// set internal function
	suite.wkr.setInternalFunctionForAction([]string{"taskLateDummy2"}, func() bool {
		defer func() {
			waitFor2ndActionInternalFunction <- struct{}{}
		}()
		return true
	})

	// set external functions
	// external pre-action func
	suite.wkr.SetExternalPreActionFunc(func() {
		waitForExternalPreActionFunction <- struct{}{}
	})
	// external function for action
	suite.wkr.SetExternalFunctionForAction("taskLateDummy2", func() (reEnqueueWorker bool) {
		defer func() {
			waitForExternalFunction2ndAction <- struct{}{}
		}()

		return true
	})
	// external post-action func
	suite.wkr.SetExternalPostActionFunc(func() {
		waitForExternalPostActionFunction <- struct{}{}
	})

	// process task2 (late-action)
	suite.wkr.ProcessTask(tsk2)

	// wait for external pre-action function
	select {
	case <-waitForExternalPreActionFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd action's pre-task function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait for internal function for action
	select {
	case <-waitFor2ndActionInternalFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd action's internal function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait for external action for function
	select {
	case <-waitForExternalFunction2ndAction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd action's function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait until external feedback function gets executed for 2nd action
	select {
	case feedback := <-waitForExternalFeedbackFunction:
		suite.Equal(suite.wkr.GetID(), feedback.workerID)
		suite.True(feedback.reEnqueueWorker)
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd action's feedback function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait until 3rd action's external post-action func gets executed
	select {
	case <-waitForExternalPostActionFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 2nd action's post-action function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// *************************************************************************
	// 3rd task: late task action + external functions + internal function
	// returning false
	// *************************************************************************

	tsk3 := &task{
		Code: taskLateAction,
		Data: action{
			Code: "taskLateDummy3",
		},
	}

	waitFor3rdActionInternalFunction := make(chan struct{}, 2)
	waitForExternalFunction3rdAction := make(chan struct{}, 2)

	// set internal function
	suite.wkr.setInternalFunctionForAction([]string{"taskLateDummy3"}, func() bool {
		defer func() {
			waitFor3rdActionInternalFunction <- struct{}{}
		}()
		return false
	})

	// set internal function
	suite.wkr.setInternalFunctionForAction([]string{"taskLateDummy3"}, func() bool {
		defer func() {
			waitFor3rdActionInternalFunction <- struct{}{}
		}()
		return false
	})

	// set external functions
	// external function for action
	suite.wkr.SetExternalFunctionForAction("taskLateDummy3", func() (reEnqueueWorker bool) {
		defer func() {
			waitForExternalFunction3rdAction <- struct{}{}
		}()

		return true
	})

	// process task3 (late-action)
	suite.wkr.ProcessTask(tsk3)

	// wait for external pre-action function
	select {
	case <-waitForExternalPreActionFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd action's pre-task function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait for internal function for action
	select {
	case <-waitFor3rdActionInternalFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd action's internal function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait for external action for function
	select {
	case <-waitForExternalFunction3rdAction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd action's function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait until external feedback function gets executed for 2nd action
	select {
	case feedback := <-waitForExternalFeedbackFunction:
		suite.Equal(suite.wkr.GetID(), feedback.workerID)
		suite.False(feedback.reEnqueueWorker)
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd action's feedback function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())

	// wait until 3rd action's external post-action func gets executed
	select {
	case <-waitForExternalPostActionFunction:
	// after 5 seconds it would fail
	case <-time.After(5 * time.Second):
		suite.FailNow("Too much time waiting for 3rd action's post-action function execution")
	}

	// metrics
	suite.Equal(0, suite.wkr.GetTaskSuccesses())
	suite.Equal(0, suite.wkr.GetTaskFailures())
}

// ***************************************************************************************
// ** Statuses. Operation in progress
// ***************************************************************************************

func (suite *WorkerTestSuite) TestSetStatusAndGetStatus() {
	suite.False(suite.wkr.getStatus("abc"))
	suite.False(suite.wkr.getStatus("abc"))
	suite.wkr.setStatus("abc", true)
	suite.True(suite.wkr.getStatus("abc"))
	suite.True(suite.wkr.getStatus("abc"))

	suite.False(suite.wkr.getStatus("abcdef"))
}

// ***************************************************************************************
// ** Metrics
// ***************************************************************************************

func (suite *WorkerTestSuite) TestGetTaskSuccesses() {
	suite.Equal(suite.wkr.taskSuccesses, suite.wkr.GetTaskSuccesses())
}

func (suite *WorkerTestSuite) TestGetTaskFailures() {
	suite.Equal(suite.wkr.taskFailures, suite.wkr.GetTaskFailures())
}

// ***************************************************************************************
// ** Run suite
// ***************************************************************************************

func TestWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerTestSuite))
}
