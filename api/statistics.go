package api

import (
	"fmt"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/logger"
	"strings"
	"sync"
)

const (
	// TOTAL_CONNECTIONS total connections open on the server at this time
	TOTAL_CONNECTIONS string = "counter-connections"
	// PERF_CONN number of current connections for a particular client
	PERF_CONN string = "perf-connections"
	// PERF_UP_COUNT current upload speed for a particular client
	PERF_UP_COUNT string = "perf-up-count"
	// PERF_DW_COUNT current download speed for a particular client
	PERF_DW_COUNT string = "perf-dw-count"
	// PERF_UP_SPEED current upload speed for a particular client
	PERF_UP_SPEED string = "perf-up-speed"
	// PERF_DW_SPEED current download speed for a particular client
	PERF_DW_SPEED string = "perf-dw-speed"
	// PERF_UP_TOTAL total number of bytes uploaded by a particular client
	PERF_UP_TOTAL string = "perf-up-total"
	// PERF_DW_TOTAL total number of bytes downloaded by a particular client
	PERF_DW_TOTAL string = "perf-dw-total"
	// INFO_PLATFORM platform used by the client, as communicated in api echo
	INFO_PLATFORM string = "info-platform"
	// INFO_ADDRESS address being sent to the client
	INFO_ADDRESS string = "info-address"
	// INFO_UPDATE last time the server received an echo from the client
	INFO_UPDATE string = "info-update"
	// INFO_OTHER_VERSION version of the software on the other end of the connection
	INFO_OTHER_VERSION string = "info-remote-version"
)

// Statistics global variable to act on statistics kept by the system
var Statistics = &statistics{}

func init() {
	Statistics.Reset()
}

// statistics struct of the statistical values that the system keeps track of, which can
// be either counters with float64 type or state string values
type statistics struct {
	// semCounters read/write mutex for counters
	semCounters sync.RWMutex
	// semState read/write mutex for states
	semState sync.RWMutex

	// counters map of the numerical values kept track of
	counters map[string]float64
	// state map of the states tracked
	state map[string]string
	// hosts cache of the destination host addresses the system is connected to
	hosts []string

	// brokerClient is the client for connection to the mqtt-based analytics service
	brokerClient *analyticsClient
	// changedCounters is the map of the modified counters in the interval to send to analytics
	changedCounters map[string]struct{}
}

// Reset called every time all the data is to thrown away and reinitialized
func (s *statistics) Reset() {
	s.semState.Lock()
	s.semCounters.Lock()
	defer func() {
		s.semState.Unlock()
		s.semCounters.Unlock()

		s.SetCounter(0.0, TOTAL_CONNECTIONS)
	}()

	logger.Debug("Statistics reset.")
	s.counters = make(map[string]float64)
	s.state = make(map[string]string)
	s.changedCounters = make(map[string]struct{})
	s.hosts = make([]string, 0, 32)
}

// Start method calls implicitly the Reset method and launches the broker client
// with the provided configuration
func (s *statistics) Start(brokerConfig *configuration.AnalyticsDefinition) {
	s.Reset()
	s.launchAnalyticsBrokerClient(brokerConfig)
}

// Stop method terminates the broker client if it is was effectively launched
func (s *statistics) Stop() {
	s.stopAnalyticsBrokerClient()
	s.brokerClient = nil
}

// asKey method abstract the generation of the attribute keys from a given prefix
// and subkeys with the following format:
// empty subkeys: "<prefix>[<subkey1>-<subkey2>-...-<subkeyN>]"
func (s *statistics) asKey(prefix string, subkeys ...string) string {
	var key = ""
	switch len(subkeys) {
	case 0:
		key = prefix + "[]"
		break
	case 1:
		key = prefix + "[" + subkeys[0] + "]"
		break
	default:
		key = prefix + "[" + strings.Join(subkeys, "-") + "]"
		break
	}
	return strings.ToLower(key)
}

// sendEvent method wraps the send operation to the analytics client
// if it is enabled / available
func (s *statistics) sendEvent(value float64, key string) {
	if s.brokerClient == nil {
		return
	}
	s.brokerClient.SendEvent(value, key)
}

// ---- Counters ---- //
// GetCounter method returns a counter with given prefix and subkeys atomically
// if it exists in the counters map, if not present or the key is empty then returns -1.
// Note that after a failed call the key will still not exist.
func (s *statistics) GetCounter(prefix string, subkeys ...string) float64 {
	key := s.asKey(prefix, subkeys...)
	if len(key) <= 2 {
		return -1
	}

	s.semCounters.RLock()
	defer s.semCounters.RUnlock()

	logger.Debug("GET counter: %s = %.2f\n", key, s.counters[key])
	if val, ok := s.counters[key]; ok {
		return val
	}
	return -1
}

// SetCounter method sets a counter with given prefix and subkeys atomically to a given value.
// Returns the set value or -1 if the key is empty. Will panic if the set value is < 0.
func (s *statistics) SetCounter(value float64, prefix string, keyparts ...string) float64 {
	if value < 0 {
		panic(fmt.Sprintf("Will not track negative values: (%s %v: %.2f)", prefix, keyparts, value))
	}

	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return -1
	}

	s.semCounters.Lock()
	defer s.semCounters.Unlock()

	s.counters[key] = value
	s.sendEvent(value, key)
	logger.Debug("SET counter: %s = %.2f\n", key, s.counters[key])
	return value
}

// GetCounterAndClear Same as GetCounter but on successful retrieval of the attribute
// value it then sets it to 0.0, so that a successive GetCounter returns 0.0 value
func (s *statistics) GetCounterAndClear(prefix string, keyparts ...string) float64 {
	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return -1
	}

	s.semCounters.Lock()
	defer s.semCounters.Unlock()

	logger.Debug("GET+CLEAR counter: %s = %.2f\n", key, s.counters[key])
	if val, ok := s.counters[key]; ok {
		s.counters[key] = 0.0
		return val
	}
	return -1
}

// IncrementCounter method sets a counter with given prefix and subkeys atomically to its current
// value plus the _incr_ parameter value (will panic if incr < 0).
// If the key does not exist than it is created atomically with the absolute value of the increment.
// Returns the set value or -1 if the key is empty.
func (s *statistics) IncrementCounter(incr float64, prefix string, keyparts ...string) float64 {
	if incr < 0.0 {
		panic("Cannot increase value by a negative value!")
	}

	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return -1
	}
	s.semCounters.Lock()
	defer s.semCounters.Unlock()

	logger.Debug("INCR counter: %s = %.2f\n", key, s.counters[key])
	value, ok := s.counters[key]
	if !ok {
		s.counters[key] = incr
		s.sendEvent(incr, key)
		return incr
	}

	s.counters[key] = value + incr
	s.sendEvent(s.counters[key], key)
	return s.counters[key]
}

// DecrementCounter method sets a counter with given prefix and subkeys atomically to its current
// value minus the _decr_ parameter value (will panic if decr < 0).
// If the key does not exist than it is created atomically with value 0.0, same thing if the current
// value minus _decr_ is less than 0.0.
// Returns the set value or empty string if the key is empty.
func (s *statistics) DecrementCounter(decr float64, prefix string, keyparts ...string) float64 {
	if decr < 0.0 {
		panic("Please specify decrement as a positive value!")
	}

	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return -1.0
	}

	s.semCounters.Lock()
	defer s.semCounters.Unlock()

	logger.Debug("DECR counter: %s = %.2f\n", key, s.counters[key])
	value, ok := s.counters[key]
	if !ok || value-1.0 < 0.0 {
		s.counters[key] = 0.0
		return 0.0
	}

	s.counters[key] = value - decr
	s.sendEvent(s.counters[key], key)
	return s.counters[key]
}

// ---- State ---- //
// GetState method returns the state string identified by the given prefix and subkeys atomically
// if it exists in the states map. If not present or the key is empty then returns -1.
// Note that after a failed call the key will still not exist.
func (s *statistics) GetState(prefix string, keyparts ...string) string {
	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return ""
	}

	s.semState.RLock()
	defer s.semState.RUnlock()

	if val, ok := s.state[key]; ok {
		return val
	}
	return ""
}

// SetCounter method sets the state identified by the given prefix and subkeys atomically to a given value.
// Returns the set value or empty string if the key is empty.
func (s *statistics) SetState(prefix, value string, keyparts ...string) string {
	key := s.asKey(prefix, keyparts...)
	if len(key) <= 2 {
		return ""
	}

	s.semState.Lock()
	defer s.semState.Unlock()

	s.state[key] = value
	return value
}

// ---- address mapping ---- //
// GetMappedAddress method returns the mapped destination address to the given source address.
// If not present or the key is empty then returns empty string.
// Note that after a failed call the key will still not exist.
func (s *statistics) GetMappedAddress(source string) string {
	s.semState.RLock()
	defer s.semState.RUnlock()

	if val, ok := s.state["src-"+source]; ok {
		return val
	}
	return ""
}

// SetMappedAddress method creates an association between the destination address and
// the given source address and then updates the hosts cache and counters accordingly.
func (s *statistics) SetMappedAddress(source string, dest string) {
	if len(source) == 0 || len(dest) == 0 {
		return
	}

	s.semState.Lock()
	defer s.semState.Unlock()

	srcKey := "src-" + source
	if _, ok := s.state[srcKey]; !ok {
		key := "host-" + dest
		s.semCounters.Lock()
		defer s.semCounters.Unlock()

		if _, found := s.counters[key]; !found {
			s.hosts = append(s.hosts, dest)
			s.counters[key] = 1
		} else {
			s.counters[key]++
		}
	}
	s.state[srcKey] = dest
}

// DeleteMappedAddress removes a source address from the mapped addresses, the referenced
// destiniation hosts are only removed from the cache if no other source map it.
func (s *statistics) DeleteMappedAddress(source string) {
	s.semState.Lock()
	defer s.semState.Unlock()

	srcKey := "src-" + source
	mapped, ok := s.state[srcKey]
	if !ok {
		// not tracked, nothing to do
		return
	}

	s.semCounters.Lock()
	defer s.semCounters.Unlock()

	key := "host-" + mapped
	if counter, found := s.counters[key]; found && counter > 1 {
		s.counters[key]--

	} else {
		delete(s.counters, key)
		for i := 0; i < len(s.hosts); i++ {
			if !strings.EqualFold(s.hosts[i], mapped) {
				continue
			}
			s.hosts = append(s.hosts[:i], s.hosts[i+1:]...)
			break
		}
	}
	delete(s.state, srcKey)
}

// ---- hosts ---- //
// GetHosts method returns the current list of destination addresses being contacted
func (s *statistics) GetHosts() []string {
	s.semState.RLock()
	defer s.semState.RUnlock()

	// for test
	// logger.Debug("hosts: %v\n", strings.Join(s.hosts, ","))
	//v := append([]string{}, "127.0.0.1")
	v := append([]string{}, s.hosts...)
	return v
}
