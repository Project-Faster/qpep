package configuration

import (
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MIN_IDLE_TIMEOUT = 1 * time.Second
	MAX_IDLE_TIMEOUT = 60 * time.Second
)

// AssertParamNumeric panics with error ErrImpossibleValidationRequested if the min and max values
// do not represent a valid range or pamics with ErrConfigurationValidationFailed if the provided
// value is not inside the range
func AssertParamNumeric(name string, value, min, max int) {
	if max < min {
		logger.Error("Validation on parameter '%s' is not possible as numeric [%d:%d]: %d\n", name, min, max, value)
		panic(errors.ErrImpossibleValidationRequested)
	}
	if value < min || value > max {
		logger.Error("Invalid parameter '%s' validated as numeric [%d:%d]: %d\n", name, min, max, value)
		panic(errors.ErrConfigurationValidationFailed)
	}
}

// AssertParamString panics with error ErrImpossibleValidationRequested if the value string provided is empty after trimming
func AssertParamString(name, value string) {
	if strings.TrimSpace(value) == "" {
		logger.Error("Validation on parameter '%s' is not possible as it empty string: %s\n", name, value)
		panic(errors.ErrConfigurationValidationFailed)
	}
}

// AssertParamChoice panics with error ErrImpossibleValidationRequested if the value string provided is not among the
// provided values
func AssertParamChoice(name, value string, choices []string) {
	if len(choices) == 0 {
		logger.Error("No valid choices provided for parameter '%s'", name)
		panic(errors.ErrImpossibleValidationRequested)
	}
	for _, c := range choices {
		if strings.EqualFold(value, c) {
			return
		}
	}
	logger.Error("Validation on parameter '%s' is not possible as it is an allowed choice: %s - %s\n", name, value, strings.Join(choices, ","))
	panic(errors.ErrConfigurationValidationFailed)
}

// AssertParamIP panics with ErrConfigurationValidationFailed if the value does not represent
// a valid ip address
func AssertParamIP(name, value string) {
	if ip := net.ParseIP(value); ip == nil {
		addr, err := net.LookupIP(value)
		if err == nil && len(addr) > 0 {
			logger.Info("Hostname '%s' validated as ip address: %s\n", value, addr[0].String())
			return
		}

		logger.Error("Invalid parameter '%s' validated as ip address: %s\n", name, value)
		panic(errors.ErrConfigurationValidationFailed)
	}
}

// AssertParamPort panics with error ErrConfigurationValidationFailed if the port value is
// not inside the expected range [1-65536] for an address port
func AssertParamPort(name string, value int) {
	if value < 1 || value > 65536 {
		logger.Error("Invalid parameter '%s' validated as port [1-65536]: %d\n", name, value)
		panic(errors.ErrConfigurationValidationFailed)
	}
}

// AssertParamPortsDifferent panics with ErrConfigurationValidationFailed if the provided list of ports contains
// duplicates or are invalid ports
func AssertParamPortsDifferent(name string, values ...int) {
	switch len(values) {
	case 0:
		return
	case 1:
		AssertParamPort(name, values[0])
		return

	case 2:
		if values[0] == values[1] {
			logger.Error("Ports '%s' must all be different: %v\n", name, values)
			panic(errors.ErrConfigurationValidationFailed)
		}
		AssertParamPort(name, values[0])
		AssertParamPort(name, values[1])
		break

	default:
		sort.Ints(values)
		AssertParamPort(name, values[0])
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				logger.Error("Ports '%s' must all be different: %v\n", name, values)
				panic(errors.ErrConfigurationValidationFailed)
			}
			AssertParamPort(name, values[i])
		}
	}
}

// AssertParamHostsDifferent panics with ErrConfigurationValidationFailed if the provided list of addresses contains
// duplicates or are invalid addresses
func AssertParamHostsDifferent(name string, values ...string) {
	switch len(values) {
	case 0:
		return
	case 1:
		AssertParamIP(name, values[0])
		return

	case 2:
		if values[0] == values[1] {
			logger.Error("Addresses '%s' must all be different: %v\n", name, values)
			panic(errors.ErrConfigurationValidationFailed)
		}
		AssertParamIP(name, values[0])
		AssertParamIP(name, values[1])
		break

	default:
		sort.Strings(values)
		AssertParamIP(name, values[0])
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				logger.Error("Addresses '%s' must all be different: %v\n", name, values)
				panic(errors.ErrConfigurationValidationFailed)
			}
			AssertParamIP(name, values[i])
		}
	}
}

func AssertParamValidTimeout(name string, value time.Duration) {
	if value < MIN_IDLE_TIMEOUT || value > MAX_IDLE_TIMEOUT {
		logger.Error("'%s' parameter not in acceptable range: %v [%v:%v]\n", name, value, MIN_IDLE_TIMEOUT, MAX_IDLE_TIMEOUT)
		panic(errors.ErrConfigurationValidationFailed)
	}
}

// addressRangesChecker struct organizes the logic for checking the speed limits
// for a certain address / range / domain
type addressRangesChecker struct {
	// incomingRanges cached ip ranges of Incoming map
	incomingRanges map[string]*net.IPNet

	// outgoingRanges cached ip ranges of Outgoing map
	outgoingRanges map[string]*net.IPNet

	// limitsCache cached values parsed from definitions
	limitsCache map[string]int64

	// lock synchronization primitive
	lock sync.RWMutex
}

var addrState addressRangesChecker

// GetAddressSpeedLimit returns the limit a value about the indicated address in incoming or outgoing direction, a value
// false indicates that the limit was never set and should be ignored
func GetAddressSpeedLimit(address net.IP, incoming bool) (int64, bool) {
	var cached map[string]*net.IPNet = nil
	var definesMap map[string]string = nil

	if addrState.limitsCache == nil {
		addrState.limitsCache = make(map[string]int64)
	}

	if incoming {
		cached = addrState.incomingRanges
		definesMap = QPepConfig.Limits.Incoming
	} else {
		cached = addrState.outgoingRanges
		definesMap = QPepConfig.Limits.Outgoing
	}
	for k, ipnet := range cached {
		if ipnet == nil {
			continue
		}
		if ipnet.Contains(address) {
			return parseSpeedLimitString(definesMap[k]), true
		}
	}
	return 0, false
}

// LoadAddressSpeedLimitMap loads the speed limit definitions in either incoming or outgoing directions
func LoadAddressSpeedLimitMap(speedMap map[string]string, incoming bool) {
	var tmpMap = make(map[string]*net.IPNet)
	for k, _ := range speedMap {
		tmpMap[k] = parseSpeedLimitAddressDefinition(k)
	}

	if incoming {
		addrState.incomingRanges = tmpMap
	} else {
		addrState.outgoingRanges = tmpMap
	}
}

// parseSpeedLimitString parses the limit string to a byte value that indicates the maximum number of bytes that
// can be transferred by a connection in a second.
// A suffix in the list (k,m,g) can be appended and indicates the value in kylobytes,megabytes or gigabytes, while
// no suffix indicates bytes number
func parseSpeedLimitString(limit string) int64 {
	limit = strings.ReplaceAll(limit, " ", "")
	if len(limit) == 0 {
		return 0
	}

	addrState.lock.RLock()
	if limitVal, ok := addrState.limitsCache[limit]; ok {
		addrState.lock.RUnlock()
		return limitVal
	}
	addrState.lock.RUnlock()

	limitNum, err := strconv.ParseInt(limit[:len(limit)-1], 10, 64)
	addrState.lock.Lock()
	defer addrState.lock.Unlock()

	if err != nil {
		addrState.limitsCache[limit] = 0
		return 0
	}

	switch limit[len(limit)-1] {
	case 'k':
		fallthrough
	case 'K':
		limitNum = limitNum * 1024
		break
	case 'm':
		fallthrough
	case 'M':
		limitNum = limitNum * 1024 * 1024
		break
	case 'g':
		fallthrough
	case 'G':
		limitNum = limitNum * 1024 * 1024 * 1024
		break
	default:
		return limitNum
	}

	addrState.limitsCache[limit] = limitNum
	return limitNum
}

// parseSpeedLimitAddressDefinition parses the passed address definition into an instance of ipnet
// that describes the corresponding subnet, the allowed values are:
// * ip address, the assigned net will contain only that address
// * cidr subnet definition: (wg. 127.0.0.1/32) which will contain the indicated ips
// * domain name: the domain name is resolved and treated as an ip definition
func parseSpeedLimitAddressDefinition(address string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(address)
	if err == nil {
		return ipnet
	}

	_, ipnet, err = net.ParseCIDR(address + "/32") // single ip
	if err == nil {
		return ipnet
	}

	if !strings.Contains(address, ":") {
		address = address + ":0"
	}
	addr, err := net.ResolveTCPAddr("tcp", address) // domain name
	if err == nil {
		_, ipnet, _ = net.ParseCIDR(addr.IP.String() + "/32")
		return ipnet
	}
	return nil
}
