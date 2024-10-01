//go:build windows && cgo

//go:generate cmd /c "robocopy x64\\ . *.lib *.sys *.dll" & exit 0

package windivert

import (
	"bufio"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/flags"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestWinDivertSuite(t *testing.T) {
	var q WinDivertSuite
	suite.Run(t, &q)
}

type WinDivertSuite struct {
	suite.Suite
}

func (s *WinDivertSuite) AfterTest(_, _ string) {
	if s.T().Skipped() {
		return
	}

	CloseWinDivertEngine()
}

func (s *WinDivertSuite) BeforeTest(_, _ string) {
	if value := os.Getenv("QPEP_CI_ENV"); len(value) > 0 {
		s.T().Skip("Skipping because CI environment does not support windivert execution")
		return
	}

	flags.Globals.Client = false
	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.General.Verbose = false
	configuration.QPepConfig.Client.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.General.APIPort = 9443
}

func (s *WinDivertSuite) SetupSuite() {
	s.T().Logf("Tests need to be run with administrator priviledges")
}

func (s *WinDivertSuite) TestInitializeWinDivertEngine() {
	t := s.T()

	itFaces, _, _ := getRouteGatewayInterfaces()

	code := InitializeWinDivertEngine(
		"127.0.0.1", "127.0.0.2",
		configuration.QPepConfig.General.APIPort, 445,
		4, itFaces[0], []int{80})

	assert.Equal(t, DIVERT_OK, code)
}

func (s *WinDivertSuite) TestInitializeWinDivertEngine_Fail() {
	t := s.T()

	itFaces, _, _ := getRouteGatewayInterfaces()

	code := InitializeWinDivertEngine(
		"127.0.0.1", "127.0.0.2",
		0, 0,
		4, itFaces[0], []int{80})

	assert.NotEqual(t, DIVERT_OK, code)
}

func (s *WinDivertSuite) TestCloseWinDivertEngine() {
	t := s.T()

	itFaces, _, _ := getRouteGatewayInterfaces()

	code := InitializeWinDivertEngine(
		"127.0.0.1", "127.0.0.2",
		configuration.QPepConfig.General.APIPort, 445,
		4, itFaces[0], []int{80})

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

// --- support methods --- //

// copy of method in gateway_interface_windows.go to workaround import cycle
func getRouteGatewayInterfaces() ([]int64, []string, error) {
	// Windows route output format is always like this:
	// Tipo pubblicazione      Prefisso met.                  Gateway idx/Nome interfaccia
	// -------  --------  ---  ------------------------  ---  ------------------------
	// No       Manuale   0    0.0.0.0/0                  18  192.168.1.1
	// No       Manuale   0    0.0.0.0/0                  20  192.168.1.1
	// No       Sistema   256  127.0.0.0/8                 1  Loopback Pseudo-Interface 1
	// No       Sistema   256  127.0.0.1/32                1  Loopback Pseudo-Interface 1
	// No       Sistema   256  127.255.255.255/32          1  Loopback Pseudo-Interface 1
	// No       Sistema   256  192.168.1.0/24             18  Wi-Fi
	// No       Sistema   256  192.168.1.0/24             20  Ethernet
	// No       Sistema   256  192.168.1.5/32             20  Ethernet
	// No       Sistema   256  192.168.1.30/32            18  Wi-Fi
	// No       Sistema   256  192.168.1.255/32           18  Wi-Fi

	// get interfaces with default routes set
	output, err, _ := shared.RunCommand("netsh", "interface", "ip", "show", "route")
	if err != nil {
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
	}

	var routeInterfaceMap = make(map[string]int64)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		value, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			continue
		}

		routeInterfaceMap[fields[4]] = value
	}
	if len(routeInterfaceMap) == 0 {
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
	}

	// get the associated names of the interfaces
	output, err, _ = shared.RunCommand("netsh", "interface", "ip", "show", "interface")
	if err != nil {
		return nil, nil, errors.ErrFailedGatewayDetect
	}

	lines = strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		key := strings.TrimSpace(fields[0])

		value, ok := routeInterfaceMap[key]
		if !ok {
			continue
		}

		delete(routeInterfaceMap, key)
		routeInterfaceMap[strings.Join(fields[4:], " ")] = value
	}

	// parse the configuration of the interfaces to extract the addresses
	output, err, _ = shared.RunCommand("netsh", "interface", "ip", "show", "config")
	if err != nil {
		logger.Error("ERR: %v", err)
		return nil, nil, errors.ErrFailedGatewayDetect
	}

	rx := regexp.MustCompile(`.+"([^"]+)"`)

	scn := bufio.NewScanner(strings.NewReader(string(output)))
	scn.Split(bufio.ScanLines)

	var interfacesList = make([]int64, 0, len(routeInterfaceMap))
	var addressesList = make([]string, 0, len(routeInterfaceMap))

BLOCK:
	for scn.Scan() {
		line := scn.Text()
		matches := rx.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		value, ok := routeInterfaceMap[matches[1]]
		if !ok {
			continue
		}

		for scn.Scan() {
			line = strings.TrimSpace(scn.Text())
			if len(line) == 0 {
				continue BLOCK
			}

			idx := strings.LastIndex(line, "IP")
			if idx != -1 {
				pieces := strings.Split(line, ":")
				if len(pieces) != 2 {
					continue
				}
				line = strings.TrimSpace(pieces[1])
				if strings.HasPrefix(line, "127.") {
					continue
				}
				addressesList = append(addressesList, line)
				interfacesList = append(interfacesList, value)
				continue BLOCK
			}
		}
	}
	return interfacesList, addressesList, nil
}
