package configuration

import (
	"fmt"
	"github.com/parvit/qpep/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"
)

func TestParamsValidation(t *testing.T) {
	var q ParamsValidationSuite
	suite.Run(t, &q)
}

type ParamsValidationSuite struct{ suite.Suite }

func (s *ParamsValidationSuite) BeforeTest(_, _ string) {
	addrState = addressRangesChecker{
		limitsCache: map[string]int64{},
	}

	QPepConfig.Limits = &LimitsDefinition{}
}

func (s *ParamsValidationSuite) TestParamsValidation_Numeric() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamNumeric("test", 1, 0, 10)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamNumeric("test", 100, 0, 10)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamNumeric("test", -100, 0, 10)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_Numeric_Invalid() {
	t := s.T()
	assert.PanicsWithValue(t, errors.ErrImpossibleValidationRequested, func() {
		AssertParamNumeric("test", 100, 10, 0)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_ValidTimeout() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamValidTimeout("test", 10*time.Second)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamValidTimeout("test", 500*time.Millisecond)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamValidTimeout("test", 2*time.Minute)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_IP() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamIP("test", "127.0.0.1")
	})
	assert.NotPanics(t, func() {
		AssertParamIP("test", "localhost")
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamIP("test", "")
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamIP("test", "ABCDEFG")
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_Port() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamPort("test", 9443)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", -200)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", 0)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPort("test", 65537)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_String() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamString("test", " value ")
	})

	assert.Panics(t, func() {
		AssertParamString("test", "  ")
	}, errors.ErrConfigurationValidationFailed)

	assert.Panics(t, func() {
		AssertParamString("test", "")
	}, errors.ErrConfigurationValidationFailed)
}

func (s *ParamsValidationSuite) TestParamsValidation_Choice() {
	t := s.T()
	assert.NotPanics(t, func() {
		AssertParamChoice("test", "value", []string{"v1", "value", "v2"})
	})
	assert.NotPanics(t, func() {
		AssertParamChoice("test", "value", []string{"v1", "value"})
	})
	assert.NotPanics(t, func() {
		AssertParamChoice("test", "value", []string{"value"})
	})

	assert.Panics(t, func() {
		AssertParamChoice("test", "not-value", []string{"v1", "value"})
	}, errors.ErrConfigurationValidationFailed)

	assert.Panics(t, func() {
		AssertParamChoice("test", "", []string{"v1", "value"})
	}, errors.ErrConfigurationValidationFailed)

	assert.Panics(t, func() {
		AssertParamChoice("test", "v2", []string{})
	}, errors.ErrImpossibleValidationRequested)
}

func (s *ParamsValidationSuite) TestParamsValidation_PortsDifferent_Valid() {
	t := s.T()
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

func (s *ParamsValidationSuite) TestParamsValidation_PortsDifferent_Fail() {
	t := s.T()
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 443, 443)
	})
	values := []int{8080}
	randNum := 8080
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, randNum)
		randNum++
	}
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", values...)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_PortsDifferent_Invalid() {
	t := s.T()
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 0)
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", 0, 0)
	})
	values := []int{-100, 0, 70000}
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamPortsDifferent("test", values...)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_HostsDifferent_Valid() {
	t := s.T()
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

func (s *ParamsValidationSuite) TestParamsValidation_HostsDifferent_Fail() {
	t := s.T()
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "127.0.0.1", "127.0.0.1")
	})
	randHost := "10.0.1.%d"
	values := []string{"10.0.1.1"}
	for i := 0; i < 1+rand.Intn(10); i++ {
		values = append(values, fmt.Sprintf(randHost, i+1))
	}
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", values...)
	})
}

func (s *ParamsValidationSuite) TestParamsValidation_HostsDifferent_Invalid() {
	t := s.T()
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "ABCD")
	})
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", "ABCD", "EFGH")
	})
	values := []string{"ABCD", "XXXX", "1234"}
	assert.PanicsWithValue(t, errors.ErrConfigurationValidationFailed, func() {
		AssertParamHostsDifferent("test", values...)
	})
}

func (s *ParamsValidationSuite) TestParseSpeedLimitAddressDefinition_SingleIP() {
	ip := parseSpeedLimitAddressDefinition("127.0.0.1")
	_, expectNet, _ := net.ParseCIDR("127.0.0.1/32")
	assert.Equal(s.T(), expectNet, ip)
}

func (s *ParamsValidationSuite) TestParseSpeedLimitAddressDefinition_Subnet() {
	ip := parseSpeedLimitAddressDefinition("127.0.0.1/16")
	_, expectNet, _ := net.ParseCIDR("127.0.0.1/16")
	assert.Equal(s.T(), expectNet, ip)
}

func (s *ParamsValidationSuite) TestParseSpeedLimitAddressDefinition_DomainFound() {
	host, _ := os.Hostname()

	addr, err := net.ResolveTCPAddr("tcp", host+":0") // domain name
	assert.Nil(s.T(), err)

	ip := parseSpeedLimitAddressDefinition(host)
	_, expectNet, _ := net.ParseCIDR(addr.IP.String() + "/32")
	assert.Equal(s.T(), expectNet, ip)
}

func (s *ParamsValidationSuite) TestParseSpeedLimitAddressDefinition_Fail() {
	ip := parseSpeedLimitAddressDefinition("test")
	assert.Nil(s.T(), ip)
}

func (s *ParamsValidationSuite) TestParseSpeedLimitString() {
	assert.Equal(s.T(), int64(0), parseSpeedLimitString(" "))
}

func (s *ParamsValidationSuite) TestParseSpeedLimitString_FailParse() {
	assert.Equal(s.T(), int64(0), parseSpeedLimitString("TEST "))
}

func (s *ParamsValidationSuite) TestParseSpeedLimitString_ParseWithSuffix() {
	assert.Equal(s.T(), int64(100*1024), parseSpeedLimitString("100 k"))
	assert.Equal(s.T(), int64(200*1024), parseSpeedLimitString("200K "))
	assert.Equal(s.T(), int64(1024*1024), parseSpeedLimitString("1 m "))
	assert.Equal(s.T(), int64(2*1024*1024), parseSpeedLimitString(" 2 M"))
	assert.Equal(s.T(), int64(1024*1024*1024), parseSpeedLimitString("1 g"))
	assert.Equal(s.T(), int64(2*1024*1024*1024), parseSpeedLimitString(" 2 G"))

	assert.Equal(s.T(), int64(1024), parseSpeedLimitString("1024 Z"))

	assert.Equal(s.T(), int64(100*1024), addrState.limitsCache["100k"])
	assert.Equal(s.T(), int64(200*1024), addrState.limitsCache["200K"])
	assert.Equal(s.T(), int64(1024*1024), addrState.limitsCache["1m"])
	assert.Equal(s.T(), int64(2*1024*1024), addrState.limitsCache["2M"])
	assert.Equal(s.T(), int64(1024*1024*1024), addrState.limitsCache["1g"])
	assert.Equal(s.T(), int64(2*1024*1024*1024), addrState.limitsCache["2G"])

	assert.Equal(s.T(), int64(0), addrState.limitsCache["1024Z"])
}

func (s *ParamsValidationSuite) TestParseSpeedLimitString_AlreadyPresent() {
	assert.Equal(s.T(), int64(100*1024), parseSpeedLimitString("100 k"))
	assert.Equal(s.T(), int64(100*1024), addrState.limitsCache["100k"])
	assert.Equal(s.T(), int64(100*1024), parseSpeedLimitString("100k"))
}

func (s *ParamsValidationSuite) TestLoadAddressSpeedLimitMap_Incoming() {
	LoadAddressSpeedLimitMap(map[string]string{
		"127.0.0.1": "100k",
	}, true)

	value, found := addrState.incomingRanges["127.0.0.1"]
	assert.True(s.T(), found)
	assert.NotNil(s.T(), value)

	value, found = addrState.outgoingRanges["127.0.0.1"]
	assert.False(s.T(), found)
	assert.Nil(s.T(), value)
}

func (s *ParamsValidationSuite) TestLoadAddressSpeedLimitMap_Outgoing() {
	LoadAddressSpeedLimitMap(map[string]string{
		"127.0.0.1": "100k",
	}, false)

	value, found := addrState.outgoingRanges["127.0.0.1"]
	assert.True(s.T(), found)
	assert.NotNil(s.T(), value)

	value, found = addrState.incomingRanges["127.0.0.1"]
	assert.False(s.T(), found)
	assert.Nil(s.T(), value)
}

func (s *ParamsValidationSuite) TestGetAddressSpeedLimit() {
	addrState.limitsCache = nil

	LoadAddressSpeedLimitMap(map[string]string{
		"127.0.0.1": "100k",
	}, true)
	LoadAddressSpeedLimitMap(map[string]string{
		"127.0.0.1": "200k",
	}, false)

	QPepConfig.Limits = &LimitsDefinition{
		Incoming: map[string]string{
			"127.0.0.1": "100k",
		},
		Outgoing: map[string]string{
			"127.0.0.1": "200k",
		},
	}

	value, found := GetAddressSpeedLimit(net.ParseIP("127.0.0.1"), true)
	assert.True(s.T(), found)
	assert.Equal(s.T(), int64(100*1024), value)

	value, found = GetAddressSpeedLimit(net.ParseIP("127.0.0.1"), false)
	assert.True(s.T(), found)
	assert.Equal(s.T(), int64(200*1024), value)
}

func (s *ParamsValidationSuite) TestGetAddressSpeedLimit_NotPresent() {
	value, found := GetAddressSpeedLimit(net.ParseIP("127.0.0.1"), true)
	assert.False(s.T(), found)
	assert.Equal(s.T(), int64(0), value)

	value, found = GetAddressSpeedLimit(net.ParseIP("127.0.0.1"), false)
	assert.False(s.T(), found)
	assert.Equal(s.T(), int64(0), value)
}
