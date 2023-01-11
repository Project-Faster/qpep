package shared

import (
	"net"
	"sort"

	. "github.com/parvit/qpep/logger"
)

func AssertParamNumeric(name string, value, min, max int) error {
	if value < min || value > max {
		Info("Invalid parameter '%s' validated as numeric [%d:%d]: %d\n", name, min, max, value)
		panic(ErrConfigurationValidationFailed)
	}
	return nil
}

func AssertParamIP(name, value string) error {
	if ip := net.ParseIP(value); ip == nil {
		Info("Invalid parameter '%s' validated as ip address: %s\n", name, value)
		panic(ErrConfigurationValidationFailed)
	}
	return nil
}

func AssertParamPort(name string, value int) error {
	if value < 1 || value > 65536 {
		Info("Invalid parameter '%s' validated as port [1-65536]: %d\n", name, value)
		panic(ErrConfigurationValidationFailed)
	}
	return nil
}

func AssertParamPortsDifferent(name string, values ...int) error {
	switch len(values) {
	case 0:
		fallthrough
	case 1:
		return nil

	case 2:
		if values[0] == values[1] {
			Info("Ports '%s' must all be different: %v\n", name, values)
			panic(ErrConfigurationValidationFailed)
		}
	default:
		sort.Ints(values)
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				Info("Ports '%s' must all be different: %v\n", name, values)
				panic(ErrConfigurationValidationFailed)
			}
		}
	}

	return nil
}

func AssertParamHostsDifferent(name string, values ...string) error {
	switch len(values) {
	case 0:
		fallthrough
	case 1:
		return nil

	case 2:
		if values[0] == values[1] {
			Info("Addresses '%s' must all be different: %v\n", name, values)
			panic(ErrConfigurationValidationFailed)
		}
	default:
		sort.Strings(values)
		for i := 1; i < len(values); i++ {
			if values[i-1] == values[i] {
				Info("Addresses '%s' must all be different: %v\n", name, values)
				panic(ErrConfigurationValidationFailed)
			}
		}
	}

	return nil
}
