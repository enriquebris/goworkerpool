// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

const (
	ErrorEmbeddedErrors                         = "embedded.errors"
	ErrorData                                   = "wrong.data"
	ErrorKillAllWorkersInProgress               = "action.kill.all.workers.in.progress"
	ErrorWaitInProgress                         = "action.wait.in.progress"
	ErrorWaitUntilInitialWorkersAreUpInProgress = "action.wait.until.initial.workers.are.up.in.progress"
	ErrorDispatcherNoActionNoTask               = "dispatcher.no.action.no.task"
	ErrorDispatcherWorkerCouldNotBeEnqueued     = "dispatcher.worker.couldnt.be.enqueued"
	ErrorNoDefaultWorkerFunction                = "no.default.worker.function"
	ErrorDispatcherChannelFull                  = "dispatcher.channel.at.full.capacity"
	ErrorFullCapacityChannel                    = "full.capacity.channel"
	ErrorWorkerNotFound                         = "worker.not.found"
	ErrorWorkerType                             = "worker.wrong.type"
	ErrorWorkerMaxTimeExceeded                  = "worker.max.time.exceeded"
	ErrorWorkerFuncPanic                        = "worker.func.panic"
)

type PoolError struct {
	code           string
	message        string
	embeddedErrors []error
}

func newPoolError(code string, message string) *PoolError {
	return &PoolError{
		code:    code,
		message: message,
	}
}

func (st *PoolError) Error() string {
	return st.message
}

func (st *PoolError) Code() string {
	return st.code
}

// AddEmbeddedErrors adds a slice of errors
func (st *PoolError) AddEmbeddedErrors(errors []error) {
	st.embeddedErrors = errors
}

func (st *PoolError) GetEmbeddedErrors() []error {
	return st.embeddedErrors
}
