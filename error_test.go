// ** goworkerpool.com  **********************************************************************************************
// ** github.com/enriquebris/goworkerpool  																			**
// ** v0.10.0  *******************************************************************************************************

package goworkerpool

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PoolErrorTestSuite struct {
	suite.Suite
}

// ***************************************************************************************
// ** Initialization
// ***************************************************************************************

// no elements at initialization
func (suite *PoolErrorTestSuite) TestInitialization() {
	queueError := newPoolError("code", "message")

	suite.Equal("code", queueError.Code())
	suite.Equal("message", queueError.Error())
}

// ***************************************************************************************
// ** Run suite
// ***************************************************************************************

func TestQueueErrorTestSuite(t *testing.T) {
	suite.Run(t, new(PoolErrorTestSuite))
}
