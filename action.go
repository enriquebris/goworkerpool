// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

const (
	actionKillWorker         = "kill.worker"
	actionLateKillWorker     = "late.kill.worker" // late kill worker
	actionLateKillAllWorkers = "late.kill.all.workers"
	actionLateKillWorkers    = "late.kill.workers"
	actionPauseAllWorkers    = "pause.all.workers"
)

// action represents an action
// An action would be normally sent to a worker, unless SendToWorker was false.
// The PreExternalFunc function (if any) would be executed by the dispatcher just before send the action to a worker. In
// case that SendToWorker was false, the dispatcher would execute the function and continue to the next iteration.
type action struct {
	// action code
	Code string
	// to let the dispatcher knows whether the action should be sent to a worker. The dispatcher would process actions
	// if they are not sent to workers.
	SendToWorker bool
	// to be executed by the dispatcher just before send the action to a worker (in case the action was to be sent to the worker)
	PreExternalFunc dispatcherGeneralFunc
}
