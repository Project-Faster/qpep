// NOTE: requires flag '-gcflags=-l' to go test to work

package shared

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func TestParamsValidation_Numeric(t *testing.T) {
	assert.NotPanics(t, func() {
		AssertParamNumeric("test", 1, 0, 10)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamNumeric("test", 100, 0, 10)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamNumeric("test", -100, 0, 10)
	})
}

func TestParamsValidation_Numeric_Invalid(t *testing.T) {
	assert.PanicsWithValue(t, ErrImpossibleValidationRequested, func() {
		AssertParamNumeric("test", 100, 10, 0)
	})
}

func TestParamsValidation_IP(t *testing.T) {
	assert.NotPanics(t, func() {
		AssertParamIP("test", "127.0.0.1")
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamIP("test", "")
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamIP("test", "ABCDEFG")
	})
}

func TestParamsValidation_Port(t *testing.T) {
	assert.NotPanics(t, func() {
		AssertParamPort("test", 9443)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", -200)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", 0)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", 65537)
	})
}

func TestParamsValidation_PortsDifferent_Valid(t *testing.T) {
	assert.NotPanics(t, func() {
		AssertParamPortsDifferent("test")
	})
	assert.NotPanics(t, func() {
		AssertParamPortsDifferent("test", 9443)
	})
	assert.NotPanics(t, func() {
		AssertParamPortsDifferent("test", 9443, 443)
	})
	values := []int{}
	randNum := 8080
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, randNum)
		randNum++
	}
	assert.NotPanics(t, func() {
		AssertParamPortsDifferent("test", values...)
	})
}

func TestParamsValidation_PortsDifferent_Fail(t *testing.T) {
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 443, 443)
	})
	values := []int{8080}
	randNum := 8080
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, randNum)
		randNum++
	}
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", values...)
	})
}

func TestParamsValidation_PortsDifferent_Invalid(t *testing.T) {
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 0)
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 0, 0)
	})
	values := []int{-100, 0, 70000}
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", values...)
	})
}

func TestParamsValidation_HostsDifferent_Valid(t *testing.T) {
	assert.NotPanics(t, func() {
		AssertParamHostsDifferent("test")
	})
	assert.NotPanics(t, func() {
		AssertParamHostsDifferent("test", "127.0.0.1")
	})
	assert.NotPanics(t, func() {
		AssertParamHostsDifferent("test", "127.0.0.1", "192.168.1.100")
	})
	values := []string{}
	randHost := "10.0.1.%d"
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, fmt.Sprintf(randHost, i))
	}
	assert.NotPanics(t, func() {
		AssertParamHostsDifferent("test", values...)
	})
}

func TestParamsValidation_HostsDifferent_Fail(t *testing.T) {
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "127.0.0.1", "127.0.0.1")
	})
	randHost := "10.0.1.%d"
	values := []string{"10.0.1.1"}
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, fmt.Sprintf(randHost, i))
	}
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", values...)
	})
}

func TestParamsValidation_HostsDifferent_Invalid(t *testing.T) {
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "ABCD")
	})
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "ABCD", "EFGH")
	})
	values := []string{"ABCD", "XXXX", "1234"}
	assert.PanicsWithValue(t, ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", values...)
	})
}
