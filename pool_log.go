// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"fmt"
	"log"
)

// Pool Log data
type PoolLog struct {
	Code    string
	Message string
	Error   error
}

const (
	logError   = "log.error"
	logChannel = "log.channel"
)

// SetLogChan sets log channel. System logs will be sent over this channel.
func (st *Pool) SetLogChan(ch chan PoolLog) {
	st.logChan = ch
}

// log logs to default output and to log channel
func (st *Pool) log(code string, message string, err error) {
	st.logToDefaultOutput(code, message, err)

	if st.logChan != nil {
		select {
		case st.logChan <- PoolLog{
			Code:    code,
			Message: message,
			Error:   err,
		}:
		default:
			st.logToDefaultOutput(logChannel, code+" - "+message, err)
		}
	}
}

// logToDefaultOutput logs to default output
func (st *Pool) logToDefaultOutput(code string, message string, err error) {
	if st.logVerbose {
		message := fmt.Sprintf("[%v] :: %v\n", code, message)
		if err != nil {
			message += fmt.Sprintf("error: %v\n", err.Error())
		}
		log.Print(message)
	}
}

// logError logs a given error
func (st *Pool) logError(message string, err error) {
	st.log(logError, message, err)
}
