package api

import (
	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestStatisticsSuite(t *testing.T) {
	var q StatisticsSuite
	suite.Run(t, &q)
}

type StatisticsSuite struct{ suite.Suite }

func (s *StatisticsSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *StatisticsSuite) BeforeTest(_, _ string) {}

func (s *StatisticsSuite) TestInit() {
	t := s.T()

	var st = &statistics{}
	assert.Nil(t, st.semCounters)
	assert.Nil(t, st.semState)
	assert.Nil(t, st.hosts)
	assert.Nil(t, st.counters)
	assert.Nil(t, st.state)

	st.init()
	assert.NotNil(t, st.semCounters)
	assert.NotNil(t, st.semState)
	assert.NotNil(t, st.hosts)
	assert.Nil(t, st.counters)
	assert.Nil(t, st.state)

	// already init coverage
	var prevCounter = st.semCounters
	var prevState = st.semState
	st.init()
	assert.NotNil(t, st.semCounters)
	assert.NotNil(t, st.semState)
	assert.Equal(t, prevCounter, st.semCounters)
	assert.Equal(t, prevState, st.semState)

	assert.NotNil(t, st.hosts)
	assert.Nil(t, st.counters)
	assert.Nil(t, st.state)
}

func (s *StatisticsSuite) TestReset() {
	t := s.T()

	var st = &statistics{}
	assert.Nil(t, st.semCounters)
	assert.Nil(t, st.semState)
	assert.Nil(t, st.hosts)
	assert.Nil(t, st.counters)
	assert.Nil(t, st.state)

	st.Reset()
	assert.NotNil(t, st.semCounters)
	assert.NotNil(t, st.semState)
	assert.NotNil(t, st.hosts)
	assert.NotNil(t, st.counters)
	assert.NotNil(t, st.state)

	// already init coverage
	var prevCounter = st.semCounters
	var prevState = st.semState
	st.Reset()
	assert.NotNil(t, st.semCounters)
	assert.NotNil(t, st.semState)
	assert.True(t, prevCounter != st.semCounters)
	assert.True(t, prevState != st.semState)

	assert.NotNil(t, st.hosts)
	assert.NotNil(t, st.counters)
	assert.NotNil(t, st.state)
}

func (s *StatisticsSuite) TestAsKey() {
	t := s.T()

	var st = &statistics{}
	assert.Equal(t, "prefix[]", st.asKey("prefix"))

	assert.Equal(t, "prefix["+TOTAL_CONNECTIONS+"]", st.asKey("prefix", TOTAL_CONNECTIONS))

	assert.Equal(t, "prefix["+TOTAL_CONNECTIONS+"-"+PERF_CONN+"]", st.asKey("prefix", TOTAL_CONNECTIONS, PERF_CONN))
}

func (s *StatisticsSuite) TestGetCounter() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.GetCounter("test", TOTAL_CONNECTIONS)
	assert.Equal(t, -1.0, counter)

	_ = st.SetCounter(1, "test", TOTAL_CONNECTIONS)

	counter = st.GetCounter("test", TOTAL_CONNECTIONS)
	assert.Equal(t, 1.0, counter)
}

func (s *StatisticsSuite) TestGetCounter_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.GetCounter("")
	assert.Equal(t, -1.0, counter)
}

func (s *StatisticsSuite) TestSetCounter() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.GetCounter("test", TOTAL_CONNECTIONS)
	assert.Equal(t, -1.0, counter)

	_ = st.SetCounter(1, "test", TOTAL_CONNECTIONS)

	counter = st.GetCounter("test", TOTAL_CONNECTIONS)
	assert.Equal(t, 1.0, counter)
}

func (s *StatisticsSuite) TestSetCounter_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.SetCounter(0.0, "")
	assert.Equal(t, -1.0, counter)
}

func (s *StatisticsSuite) TestSetCounter_BadValue() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Panics(t, func() {
		st.SetCounter(-1.0, "test")
	})
}

func (s *StatisticsSuite) TestGetCounterAndClear() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.GetCounterAndClear("test", TOTAL_CONNECTIONS)
	assert.Equal(t, -1.0, counter)

	_ = st.SetCounter(1, "test", TOTAL_CONNECTIONS)

	counter = st.GetCounterAndClear("test", TOTAL_CONNECTIONS)
	assert.Equal(t, 1.0, counter)

	// after first get the value was cleared
	counter = st.GetCounterAndClear("test", TOTAL_CONNECTIONS)
	assert.Equal(t, 0.0, counter)
}

func (s *StatisticsSuite) TestGetCounterAndClear_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.GetCounterAndClear("")
	assert.Equal(t, -1.0, counter)
}

func (s *StatisticsSuite) TestIncrementCounter() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.IncrementCounter(1.0, "test", TOTAL_CONNECTIONS)
	assert.Equal(t, 1.0, counter)

	counter = st.IncrementCounter(1.0, "test", TOTAL_CONNECTIONS)
	assert.Equal(t, 2.0, counter)
}

func (s *StatisticsSuite) TestIncrementCounter_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.IncrementCounter(0.0, "")
	assert.Equal(t, -1.0, counter)
}

func (s *StatisticsSuite) TestIncrementCounter_BadValue() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Panics(t, func() {
		st.IncrementCounter(-1.0, "test")
	})
}

func (s *StatisticsSuite) TestDecrementCounter() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	_ = st.SetCounter(1.0, "test", TOTAL_CONNECTIONS)

	counter := st.DecrementCounter(1.0, "test", TOTAL_CONNECTIONS)
	assert.Equal(t, 0.0, counter)

	counter = st.DecrementCounter(1.0, "test", TOTAL_CONNECTIONS)
	assert.Equal(t, 0.0, counter)
}

func (s *StatisticsSuite) TestDecrementCounter_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	counter := st.DecrementCounter(0.0, "")
	assert.Equal(t, -1.0, counter)
}

func (s *StatisticsSuite) TestDecrementCounter_BadValue() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Panics(t, func() {
		st.DecrementCounter(-1.0, "test")
	})
}

func (s *StatisticsSuite) TestGetSetState() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetState("test", TOTAL_CONNECTIONS))
	assert.Equal(t, "X", st.SetState("X", "test", TOTAL_CONNECTIONS))
	assert.Equal(t, "X", st.GetState("test", TOTAL_CONNECTIONS))
}

func (s *StatisticsSuite) TestGetSetState_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetState(""))
	assert.Equal(t, "", st.SetState("X", ""))
}

func (s *StatisticsSuite) TestGetMappedAddress() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetMappedAddress("127.0.0.1"))
	st.SetMappedAddress("127.0.0.1", "8.8.8.8")
	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("127.0.0.1"))
}

func (s *StatisticsSuite) TestSetMappedAddress() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetMappedAddress("127.0.0.1"))
	st.SetMappedAddress("127.0.0.1", "8.8.8.8")
	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("127.0.0.1"))

	st.SetMappedAddress("192.168.1.1", "8.8.8.8")
	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("192.168.1.1"))
}

func (s *StatisticsSuite) TestDeleteMappedAddress() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetMappedAddress("127.0.0.1"))
	st.SetMappedAddress("127.0.0.1", "8.8.8.8")
	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("127.0.0.1"))

	st.SetMappedAddress("192.168.1.1", "8.8.8.8")
	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("192.168.1.1"))

	st.DeleteMappedAddress("127.0.0.1")
	assert.Equal(t, "", st.GetMappedAddress("127.0.0.1"))

	assert.Equal(t, "8.8.8.8", st.GetMappedAddress("192.168.1.1"))
	st.DeleteMappedAddress("192.168.1.1")
	assert.Equal(t, "", st.GetMappedAddress("192.168.1.1"))
}

func (s *StatisticsSuite) TestGetHosts() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	st.SetMappedAddress("127.0.0.1", "8.8.8.8")
	st.SetMappedAddress("192.168.1.1", "8.8.8.8")

	assertArrayEqualsString(t, []string{"8.8.8.8"}, st.GetHosts())

	st.DeleteMappedAddress("127.0.0.1")
	assertArrayEqualsString(t, []string{"8.8.8.8"}, st.GetHosts())

	st.DeleteMappedAddress("192.168.1.1")
	assertArrayEqualsString(t, []string{}, st.GetHosts())
}

// --- Utils --- //
func assertArrayEqualsString(t *testing.T, vec_a, vec_b []string) {
	assert.Equal(t, len(vec_a), len(vec_b))
	if t.Failed() {
		t.Logf("a: %v, b: %v\n", vec_a, vec_b)
		return
	}

	for i := 0; i < len(vec_a); i++ {
		assert.Equal(t, vec_a[i], vec_b[i])
		if t.Failed() {
			t.Logf("a: %v, b: %v\n", vec_a, vec_b)
			return
		}
	}
}
