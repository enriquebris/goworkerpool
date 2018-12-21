package goworkerpool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	initialWorkers = 10
)

type PoolTestSuite struct {
	suite.Suite
	pool *Pool
}

func (suite *PoolTestSuite) SetupTest() {
	suite.pool = NewPool(initialWorkers, 10, false)
}

// ***************************************************************************************
// ** initialization
// ***************************************************************************************

// no worker's func is expected just after the pool is created
func (suite *PoolTestSuite) TestInitializationNoWorkerFunc() {
	suite.Nil(suite.pool.fn, "Worker's func must be nil at initialization")
}

// no workers are expected to be running just after the pool is created
func (suite *PoolTestSuite) TestInitializationNoWorkersUp() {
	suite.Equal(suite.pool.GetTotalWorkers(), 0, "No workers should be up at initialization.")
}

// success / fail counters are expected to be zero just after the pool is created
func (suite *PoolTestSuite) TestInitializationCountersEqualToZero() {
	suite.Equal(suite.pool.fnSuccessCounter, 0, "Success counter should be zero at initialization")
	suite.Equal(suite.pool.fnFailCounter, 0, "Fail counter should be zero at initialization")
}

// workers to start up by StartWorkers() must be the same amount passed to initialize(initialWorkers int, ...)
func (suite *PoolTestSuite) TestInitializationNumberOfWorkers() {
	suite.Equalf(suite.pool.initialWorkers, initialWorkers, "Number of workers to start up must be: %v", initialWorkers)
}

// newWorkerChan (channel to send signals on each new worker spin up) must be nil at initialization
func (suite *PoolTestSuite) TestSetNewWorkerChanNoChannel() {
	suite.Nil(suite.pool.newWorkerChan, "newWorkerChan must be nil at initialization")
}

// newWorkerChan (channel to send signals on each new worker spin up) == chan (from SetNewWorkerChan(chan) ) ==> no error
func (suite *PoolTestSuite) TestSetNewWorkerChan() {
	tmpChan := make(chan int)
	suite.pool.SetNewWorkerChan(tmpChan)
	suite.Equal(tmpChan, suite.pool.newWorkerChan, "newWorkerChan must be equal to the chan passed as param in SetNewWorkerChan(chan)")
}

// ***************************************************************************************
// ** StartWorkers()
// ***************************************************************************************

// StartWorkers() with a worker's func at first invoke ==> no error
func (suite *PoolTestSuite) TestStartWorkersNoErrors() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })
	suite.NoError(suite.pool.StartWorkers(), "StartWorkers() must return no error the first time it is invoked.")
}

// verifies that the exact amount of workers (initialWorkers) are started up, not more, not less
func (suite *PoolTestSuite) TestStartWorkersExactAmountOfWorkers() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })
	// set newWorkerChan to receive new workers' signals
	newWorkerChan := make(chan int, 10)
	suite.pool.SetNewWorkerChan(newWorkerChan)
	// start up the workers
	suite.pool.StartWorkers()

	totalWorkersUp := 0
	keepWorking := true
	for keepWorking {
		select {
		case newWorker := <-newWorkerChan:
			totalWorkersUp += newWorker
			if totalWorkersUp == initialWorkers {
				// OK !!!!
				keepWorking = false
			}

		// This test will fail if the following amount of time is exceeded after the last worker was started up.
		// The idea is: the amount of time between two workers initialization should be less than 3 seconds.
		// Keep in mind that this amount of time will depend on the host speed.
		case <-time.After(3 * time.Second):
			suite.Failf("StartWorkers() started up less workers than expected", "Expected new workers: %v. Total workers started up: %v.", initialWorkers, totalWorkersUp)
			keepWorking = false
		}
	}

	// TODO ::: Check for a better way to verify that no extra workers are started up
	// verify that no more workers than initialWorkers are started up
	if totalWorkersUp == initialWorkers {
		select {
		case newWorker := <-newWorkerChan:
			suite.Failf("StartWorkers() started up more workers than expected", "Expected workers: %v. Total workers started up: %v.", initialWorkers, initialWorkers+newWorker)

		// wait 3 seconds to verify that no extra workers are started up
		case <-time.After(3 * time.Second):
			break
		}
	}
}

// ***************************************************************************************
// ** StartWorkersAndWait()
// ***************************************************************************************

// StartWorkersAndWait() with a worker's func at first invoke ==> no error
func (suite *PoolTestSuite) TestStartWorkersAndWaitWithWorkerHandlerFunction() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })
	suite.NoError(suite.pool.StartWorkersAndWait(), "StartWorkersAndWait() must return no error having a defined worker's handler function.")
}

// StartWorkersAndWait() without a worker's func at first invoke ==> error
func (suite *PoolTestSuite) TestStartWorkersAndWaitNoWorkerHandlerFunction() {
	suite.Error(suite.pool.StartWorkersAndWait(), "StartWorkersAndWait() must return error if no worker's handler func is defined.")
}

// verifies that the exact amount of workers (initialWorkers) are started up, not more, not less && the elapsed time between each worker initialization
func (suite *PoolTestSuite) TestStartWorkersAndWait() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })

	newWorkerChan := make(chan int, initialWorkers * 2)
	suite.pool.SetNewWorkerChan(newWorkerChan)

	// start listening for the "new worker" signals
	go func() {
		totalWorkersUp := 0
		select {
		case message := <-newWorkerChan:
			totalWorkersUp += message
			if totalWorkersUp == initialWorkers {
				break
			}
		case <-time.After(2 * time.Second):
			suite.True(false, "Too much time waiting for the workers")
		}
	}()

	// start up all workers and wait until them are 100% alive
	suite.NoError(suite.pool.StartWorkersAndWait(), "Unexpected error fromStartWorkersAndWait()")

	totalWorkers := suite.pool.GetTotalWorkers()
	suite.Equalf(totalWorkers, initialWorkers, "Expected workers up: %v, Total workers up: %v", initialWorkers, totalWorkers)
}

// ***************************************************************************************
// ** Wait()
// ***************************************************************************************

// Wait() without worker's func (worker's handler function) ==> error
func (suite *PoolTestSuite) TestWaitWithoutWorkerFunc() {
	suite.Error(suite.pool.Wait(), "Wait() must return error if no worker's func is defined.")
}

// Wait() with worker's func but before start up workers ==> no error
func (suite *PoolTestSuite) TestWaitBeforeStartWorkers() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })
	suite.NoError(suite.pool.Wait(), "Wait() must return nil if no workers are running.")
}

// Wait() with worker's func and zero workers up ==> nil
func (suite *PoolTestSuite) TestWaitWithZeroWorkers() {
	// add a dummy worker's func
	suite.pool.SetWorkerFunc(func(data interface{}) bool { return true })
	// mimic total workers == 0
	suite.pool.totalWorkers = 0

	suite.Nil(suite.pool.Wait(), "Wait() must return nil if no workers are running.")
}

// ***************************************************************************************
// ** WaitUntilNSuccesses(n)
// ***************************************************************************************

// WaitUntilNSuccesses(n) without worker's func (worker's handler function) ==> error
func (suite *PoolTestSuite) TestWaitUntilNSuccessesWithoutWorkerFunc() {
	suite.Error(suite.pool.WaitUntilNSuccesses(5), "WaitUntilNSuccesses(n) must return error if no worker's func is defined.")
}

// WaitUntilNSuccesses(n) ==> successes (workers returning true) must be >= n
func (suite *PoolTestSuite) TestWaitUntilNSuccesses() {
	// channel to receive successes
	successChan := make(chan bool, 5)
	successCounter := 0
	minSuccesses := 3

	// the worker's handler func will send always a signal over the given channel
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		successChan <- true
		return true
	})

	// goroutine to catch the signals sent by the workers
	go func(ch chan bool, counter *int) {
		for {
			select {
			case <-ch:
				*counter++
			}
		}
	}(successChan, &successCounter)

	// start the workers
	suite.pool.StartWorkers()
	// enqueue minSuccesses + 1 jobs
	for i := 0; i < minSuccesses+1; i++ {
		suite.pool.AddTask(nil)
	}
	// wait until minSuccesses jobs were successfully processed
	suite.pool.WaitUntilNSuccesses(minSuccesses)

	suite.Truef(successCounter >= minSuccesses, "At least %v jobs have to be successfully processed by the workers, there are only %v", minSuccesses, successCounter)
}

// ***************************************************************************************
// ** KillWorker()
// ***************************************************************************************

// KillWorker() ==> error if KillAllWorkersInCourse() in course
func (suite *PoolTestSuite) TestKillWorkerWhileKillAllWorkersInCourse() {
	suite.pool.broadMessages.Store(broadMessageKillAllWorkers, true)
	suite.Error(suite.pool.KillWorker(), "KillWorker() must return error while KillAllWorkers() in course.")
}

// KillWorker() ==> nil if KillAllWorkersInCourse() is not in course
func (suite *PoolTestSuite) TestKillWorkerWhileNotKillAllWorkersInCourse() {
	suite.pool.broadMessages.Delete(broadMessageKillAllWorkers)
	suite.NoError(suite.pool.KillWorker(), "KillWorker() must not return an error while KillAllWorkers() is not in course.")
}

// only one "killed worker" confirmation must be received after KillWorker()
func (suite *PoolTestSuite) TestKillWorker() {
	// dummy worker's handler func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// channel to receive "new worker" signals
	ch := make(chan int, initialWorkers)
	suite.pool.SetNewWorkerChan(ch)

	// start the workers
	suite.pool.StartWorkers()

	// wait for the first worker up
	select {
	case <-ch:
		break
	case <-time.After(3 * time.Second):
		suite.Fail("Too long to spin up workers")
	}

	// channel to receive "killed worker" signals
	ch = make(chan int, initialWorkers)
	suite.pool.SetKilledWorkerChan(ch)

	// kill a worker
	err := suite.pool.KillWorker()
	suite.NoError(err, "Error trying to kill a worker: '%v'", err)

	// wait for the "worker killed" signal
	select {
	case total := <-ch:
		suite.Truef(total == 1, "%v workers killed, expected: 1", total)
	case <-time.After(3 * time.Second):
		suite.Fail("Too long time waiting for the \"killed worker\" signal")
	}

	// no more "worker killed" signal should be received
	select {
	case total := <-ch:
		suite.Failf("Extra workers killed", "%v Extra confirmations received", total)
	case <-time.After(3 * time.Second):
		// it's ok, no more signals received
		break
	}
}

// ***************************************************************************************
// ** KillWorkers(n)
// ***************************************************************************************

// KillWorkers(n) ==> error if KillAllWorkersInCourse() in course
func (suite *PoolTestSuite) TestKillWorkersWhileKillAllWorkersInCourse() {
	suite.pool.broadMessages.Store(broadMessageKillAllWorkers, true)
	suite.Error(suite.pool.KillWorkers(5), "KillWorkers(n) must return error while KillAllWorkers() in course.")
}

// KillWorkers(n) ==> nil if KillAllWorkersInCourse() is not in course
func (suite *PoolTestSuite) TestKillWorkersWhileNotKillAllWorkersInCourse() {
	suite.pool.broadMessages.Delete(broadMessageKillAllWorkers)
	suite.NoError(suite.pool.KillWorkers(5), "KillWorkers(n) must not return an error while KillAllWorkers() is not in course.")
}

/*
// only n "killed worker" confirmations must be received after KillWorkers(n)
func (suite *PoolTestSuite) TestKillWorkers() {
	// dummy worker's handler func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// channel to receive "new worker" signals
	ch := make(chan int, initialWorkers)
	suite.pool.SetNewWorkerChan(ch)

	// start the workers
	suite.pool.StartWorkers()

	// wait for the first worker up
	select {
	case <-ch:
		break
	case <-time.After(3 * time.Second):
		suite.Fail("Too long to spin up workers")
	}

	// channel to receive "killed worker" signals
	ch = make(chan int, initialWorkers)
	suite.pool.SetKilledWorkerChan(ch)

	// kill a worker
	err := suite.pool.KillWorker()
	suite.NoError(err, "Error trying to kill a worker: '%v'", err)

	// wait for the "worker killed" signal
	select {
	case total := <-ch:
		suite.Truef(total == 1, "%v workers killed, expected: 1", total)
	case <-time.After(3 * time.Second):
		suite.Fail("Too long time waiting for the \"killed worker\" signal")
	}

	// no more "worker killed" signal should be received
	select {
	case total := <-ch:
		suite.Failf("Extra workers killed", "%v Extra confirmations received", total)
	case <-time.After(3 * time.Second):
		// it's ok, no more signals received
		break
	}
}
*/

// ***************************************************************************************
// ** KillAllWorkersAndWait()
// ***************************************************************************************

// worker must be 0 after KillAllWorkersAndWait() (total workers == initialWorkers before invoke KillAllWorkersAndWait())
func (suite *PoolTestSuite) TestKillAllWorkersAndWait() {
	// dummy worker's handler func
	suite.pool.SetWorkerFunc(func(data interface{}) bool {
		return true
	})

	// start the workers
	suite.pool.StartWorkers()
	// send the "kill all workers" signal and wait until it takes effect
	suite.pool.KillAllWorkersAndWait()
	// workers must be 0
	suite.Truef(suite.pool.GetTotalWorkers() == 0, "No workers should be up after a KillAllWorkersAndWait(). There are %v workers up.", suite.pool.GetTotalWorkers())
}

// worker must be 0 after KillAllWorkersAndWait() (total workers == 0 before invoke KillAllWorkersAndWait())
func (suite *PoolTestSuite) TestKillAllWorkersAndWaitWithZeroWorkers() {
	// send the "kill all workers" signal and wait until it takes effect
	suite.pool.KillAllWorkersAndWait()

	// workers must be 0
	suite.Truef(suite.pool.GetTotalWorkers() == 0, "No workers should be up after a KillAllWorkersAndWait(). There are %v workers up.", suite.pool.GetTotalWorkers())
}

// ***************************************************************************************
// ** SetTotalWorkers(n)
// ***************************************************************************************

// SetTotalWorkers(n) before start workers ==> error
func (suite *PoolTestSuite) TestSetTotalWorkersBeforeStartWorkers() {
	suite.Error(suite.pool.SetTotalWorkers(10), "SetTotalWorkers(n) must return error if no workers were started.")
}

// SetTotalWorkers(n) while StartWorkers is still running => error
func (suite *PoolTestSuite) TestSetTotalWorkersWhileStartWorkersIsStillRunning() {
	suite.pool.StartWorkers()
	// ensure that StartWorkers() is still running while SetTotalWorkers() is invoked
	// suite.pool.workersStarted == false ==> StartWorkers() is still running or it hasn't invoked yet
	suite.Equal(suite.pool.workersStarted == false && suite.pool.SetTotalWorkers(10) != nil, true, "SetTotalWorkers(n) must return error if StartWorkers() is in progress.")
}

// SetTotalWorkers() after StartWorkers() and while KillAllWorkers() is in progress ==> error
func (suite *PoolTestSuite) TestSetTotalWorkersDuringKillAllWorkers() {
	// mimic pool.StartWorkers()
	suite.pool.workersStarted = true
	// internal message to "kill all workers", it is triggered by KillAllWorkers()
	suite.pool.broadMessages.Store(broadMessageKillAllWorkers, true)
	suite.Error(suite.pool.SetTotalWorkers(10), "SetTotalWorkers(n) must return error while KillAllWorkers() is in progress.")
}

// SetTotalWorkers() after StartWorkers() and while KillAllWorkers() is NOT in progress ==> not error
func (suite *PoolTestSuite) TestSetTotalWorkers() {
	// mimic pool.StartWorkers()
	suite.pool.workersStarted = true
	// ensure that KillAllWorkers() is not in progress
	suite.pool.broadMessages.Store(broadMessageKillAllWorkers, false)

	suite.Nil(suite.pool.SetTotalWorkers(10), "SetTotalWorkers(n) must return nil if workers were started and KillAllWorkers() is not in progress.")
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(PoolTestSuite))
}

// ***************************************************************************************
// ** Helpers
// ***************************************************************************************

func (suite *PoolTestSuite) waitUntilStartWorkers() {
	//suite.NoError(suite.pool.StartWorkers(), "StartWorkers() must return")
}
