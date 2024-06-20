//go:build windows && cgo

//go:generate cmd /c "robocopy x64\\ . *.lib *.sys *.dll" & exit 0

package windivert

import (
	"context"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/flags"
	"github.com/parvit/qpep/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"time"
)

func TestWinDivertSuite(t *testing.T) {
	var q WinDivertSuite
	suite.Run(t, &q)
}

type WinDivertSuite struct {
	suite.Suite

	ctx      context.Context
	cancel   context.CancelFunc
	finished bool
}

func (s *WinDivertSuite) AfterTest(_, _ string) {
	if s.T().Skipped() {
		return
	}

	CloseWinDivertEngine()

	// stops the launched server
	s.cancel()
	<-time.After(1 * time.Second)
	assert.True(s.T(), s.finished)
}

func (s *WinDivertSuite) BeforeTest(_, _ string) {
	if value := os.Getenv("QPEP_CI_ENV"); len(value) > 0 {
		s.T().Skip("Skipping because CI environment does not support windivert execution")
		return
	}

	flags.Globals.Client = false
	shared.QPepConfig.Verbose = true
	shared.QPepConfig.ListenHost = "127.0.0.1"
	shared.QPepConfig.GatewayAPIPort = 9443
	s.finished = false
	s.ctx, s.cancel = context.WithCancel(context.Background())

	go func() {
		api.RunServer(s.ctx, s.cancel, true)
		s.finished = true
	}()

	<-time.After(1 * time.Second)
	assert.False(s.T(), s.finished)
}

func (s *WinDivertSuite) SetupSuite() {
	s.T().Logf("Tests need to be run with administrator priviledges")
}

func (s *WinDivertSuite) TestInitializeWinDivertEngine() {
	t := s.T()

	addr, itFaces := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	code := InitializeWinDivertEngine(
		addr, addr,
		shared.QPepConfig.GatewayAPIPort, 445,
		4, itFaces[0])

	assert.Equal(t, DIVERT_OK, code)
}

func (s *WinDivertSuite) TestInitializeWinDivertEngine_Fail() {
	t := s.T()

	addr, itFaces := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	code := InitializeWinDivertEngine(
		addr, addr,
		0, 0,
		4, itFaces[0])

	assert.NotEqual(t, DIVERT_OK, code)
}

func (s *WinDivertSuite) TestCloseWinDivertEngine() {
	t := s.T()

	addr, itFaces := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	code := InitializeWinDivertEngine(
		addr, addr,
		shared.QPepConfig.GatewayAPIPort, 445,
		4, itFaces[0])

	assert.Equal(t, DIVERT_OK, code)

	code = CloseWinDivertEngine()
	assert.Equal(t, DIVERT_OK, code)
}

func (s *WinDivertSuite) TestEnableDiverterLogging() {
	EnableDiverterLogging(true)
	EnableDiverterLogging(false)

	// only for coverage, no actual effect can be checked
}

func (s *WinDivertSuite) TestGetConnectionStateDate_Closed() {
	code, srcPort, dstPort, srcAddress, dstAddress := GetConnectionStateData(9999)
	assert.Equal(s.T(), DIVERT_ERROR_NOT_OPEN, code)

	assert.Equal(s.T(), srcPort, -1)
	assert.Equal(s.T(), dstPort, -1)
	assert.Equal(s.T(), srcAddress, "")
	assert.Equal(s.T(), dstAddress, "")
}
