package shared

import (
	"net"
	"sort"

	. "github.com/parvit/qpep/logger"
)

func AssertParamNumeric(name string, value, min, max int) {
	if max < min {
		Error("Validation on parameter '%s' is not possible as numeric [%d:%d]: %d\n", name, min, max, value)
		panic(ErrImpossibleValidationRequested)
	}
	if value < min || value > max {
		Error("Invalid parameter '%s' validated as numeric [%d:%d]: %d\n", name, min, max, value)
		panic(ErrConfigurationValidationFailed)
	}
}

func AssertParamIP(name, value string) {
	if ip := net.ParseIP(value); ip == nil {
		Info("Invalid parameter '%s' validated as ip address: %s\n", name, value)
		panic(ErrConfigurationValidationFailed)
	}
}

func AssertParamPort(name string, value int) {
	if value < 1 || value > 65536 {
		Info("Invalid parameter '%s' validated as port [1-65536]: %d\n", name, value)
		panic(ErrConfigurationValidationFailed)
	}
}

func AssertParamPortsDifferent(name string, values ...int) {
	switch len(values) {
	case 0:
		return
	case 1:
		AssertParamPort(name, values[0])
		return

	case 2:
		if values[0] == values[1] {
			Info("Ports '%s' must all be different: %v\n", name, values)
			panic(ErrConfigurationValidationFailed)
		}
		AssertParamPort(name, values[0])
		AssertParamPort(name, values[1])
		break

	default:
		sort.Ints(values)
		AssertParamPort(name, values[0])
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				Info("Ports '%s' must all be different: %v\n", name, values)
				panic(ErrConfigurationValidationFailed)
			}
			AssertParamPort(name, values[i])
		}
	}
}

func AssertParamHostsDifferent(name string, values ...string) {
	switch len(values) {
	case 0:
		return
	case 1:
		AssertParamIP(name, values[0])
		return

	case 2:
		if values[0] == values[1] {
			Info("Addresses '%s' must all be different: %v\n", name, values)
			panic(ErrConfigurationValidationFailed)
		}
		AssertParamIP(name, values[0])
		AssertParamIP(name, values[1])
		break

	default:
		sort.Strings(values)
		AssertParamIP(name, values[0])
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				Info("Addresses '%s' must all be different: %v\n", name, values)
				panic(ErrConfigurationValidationFailed)
			}
			AssertParamIP(name, values[i])
		}
	}
}

func CheckAddressSpeedLimit(address string) (int, bool) {
	return 0, false //TODO
}
