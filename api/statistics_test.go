package api

import (
	"bou.ke/monkey"
	"github.com/parvit/qpep/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestStatisticsSuite(t *testing.T) {
	var q StatisticsSuite
	suite.Run(t, &q)
}

type StatisticsSuite struct{ suite.Suite }

func (s *StatisticsSuite) AfterTest(_, _ string) {
	Statistics.Stop()
	monkey.UnpatchAll()
}

func (s *StatisticsSuite) BeforeTest(_, testName string) {
	if testName == "TestParallelExecution" {
		return
	}
	Statistics.Start(&shared.AnalyticsDefinition{
		Enabled:        true,
		BrokerAddress:  "127.0.0.1",
		BrokerPort:     1883,
		BrokerProtocol: "tcp",
		BrokerTopic:    "topic/test",
	})
}

func (s *StatisticsSuite) TestReset() {
	t := s.T()

	var st = &statistics{}
	assert.Nil(t, st.hosts)
	assert.Nil(t, st.counters)
	assert.Nil(t, st.state)

	st.Reset()
	assert.NotNil(t, st.hosts)
	assert.NotNil(t, st.counters)
	assert.NotNil(t, st.state)

	assert.Equal(s.T(), 0.0, st.GetCounter(TOTAL_CONNECTIONS))
}

func (s *StatisticsSuite) TestStartStop() {
	Statistics.Start(&shared.AnalyticsDefinition{
		Enabled:        true,
		BrokerAddress:  "127.0.0.1",
		BrokerPort:     1883,
		BrokerProtocol: "tcp",
		BrokerTopic:    "topic/test",
	})
	assert.NotNil(s.T(), Statistics.brokerClient)

	Statistics.Stop()
	assert.Nil(s.T(), Statistics.brokerClient)
}

func (s *StatisticsSuite) TestAsKey() {
	t := s.T()

	var st = &statistics{}
	assert.Equal(t, "prefix[]", st.asKey("prefix"))

	assert.Equal(t, "prefix["+TOTAL_CONNECTIONS+"]", st.asKey("prefix", TOTAL_CONNECTIONS))

	assert.Equal(t, "prefix["+TOTAL_CONNECTIONS+"-"+PERF_CONN+"]", st.asKey("prefix", TOTAL_CONNECTIONS, PERF_CONN))
}

func (s *StatisticsSuite) TestSendEvent() {
	t := s.T()

	var st = &statistics{
		brokerClient: &analyticsClient{},
	}

	// test with enabled broker
	var receivedName = ""
	var receivedValue = 0.0
	client := monkey.PatchInstanceMethod(reflect.TypeOf(st.brokerClient),
		"SendEvent",
		func(_ *analyticsClient, value float64, name string) {
			receivedName = name
			receivedValue = value
		})
	defer client.Unpatch()

	st.sendEvent(101.0, "test")

	assert.Equal(t, "test", receivedName)
	assert.Equal(t, 101.0, receivedValue)

	// test with disabled broker
	receivedName = ""
	receivedValue = 0.0

	st.brokerClient = nil
	assert.NotPanics(t, func() {
		st.sendEvent(101.0, "test")
	})

	assert.Equal(t, "", receivedName)
	assert.Equal(t, 0.0, receivedValue)
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
	assert.Equal(t, "X", st.SetState("test", "X", TOTAL_CONNECTIONS))
	assert.Equal(t, "X", st.GetState("test", TOTAL_CONNECTIONS))
}

func (s *StatisticsSuite) TestGetSetState_BadKey() {
	t := s.T()

	var st = &statistics{}
	st.Reset()

	assert.Equal(t, "", st.GetState(""))
	assert.Equal(t, "", st.SetState("", "X"))
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

	st.SetMappedAddress("", "8.8.8.8")
	st.SetMappedAddress("127.0.0.1", "")

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

	// present and then removed
	st.DeleteMappedAddress("127.0.0.1")
	assert.Equal(t, "", st.GetMappedAddress("127.0.0.1"))

	// not present
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

func (s *StatisticsSuite) TestParallelExecution() {
	Statistics.Reset()

	wg := &sync.WaitGroup{}
	wg.Add(14)

	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.Reset()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.SetCounter(10000.0, "cnt", "1", "2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.GetCounter("cnt", "1", "2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.GetCounterAndClear("cnt", "1", "2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.IncrementCounter(2.0, "cnt", "1", "2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.DecrementCounter(2.0, "cnt", "1", "2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.SetMappedAddress("127.0.0.1", "127.0.0.2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.GetMappedAddress("127.0.0.1")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.DeleteMappedAddress("127.0.0.1")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.SetState("st1", "127.0.0.2")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.GetState("st1")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			Statistics.GetHosts()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			var count = 0
			Statistics.semCounters.RLock()
			for range Statistics.counters {
				count++
			}
			Statistics.semCounters.RUnlock()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			var count = 0
			Statistics.semState.RLock()
			for range Statistics.state {
				count++
			}
			Statistics.semState.RUnlock()
		}
	}()

	var finished = false

	// If any goroutine remains hanged then this will unblock the waitgroup
	go func() {
		<-time.After(10 * time.Second)
		if finished {
			return
		}
		for !finished {
			func() {
				defer func() {
					if err := recover(); err != nil {
						finished = true
					}
				}()
				wg.Done()
			}()
		}
		s.T().FailNow()
	}()

	wg.Wait()

	finished = true
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
