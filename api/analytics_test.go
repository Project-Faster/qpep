package api

import (
	"bou.ke/monkey"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/logger"
	"github.com/parvit/qpep/shared/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnalyticsSuite(t *testing.T) {
	port, _ := strconv.ParseInt(os.Getenv("QPEP_ENV_BROKER_PORT"), 10, 64)

	var q = AnalyticsSuite{
		brokerAddress: os.Getenv("QPEP_ENV_BROKER_ADDRESS"),
		brokerTopic:   os.Getenv("QPEP_ENV_BROKER_TOPIC"),
		brokerPort:    int(port),
		brokerProto:   os.Getenv("QPEP_ENV_BROKER_PROTO"),
	}
	if q.brokerAddress == "" {
		q.brokerAddress = "127.0.0.1"
	}
	if q.brokerProto == "" {
		q.brokerProto = "tcp"
	}
	if port <= 0 || port > 65535 {
		q.brokerPort = 1883
	}
	if q.brokerTopic == "" {
		q.brokerTopic = "test/topic"
	}
	suite.Run(t, &q)
}

type AnalyticsSuite struct {
	suite.Suite

	brokerAddress string
	brokerPort    int
	brokerProto   string
	brokerTopic   string
}

func (s *AnalyticsSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *AnalyticsSuite) BeforeTest(_, _ string) {
	Statistics.Reset()
	Statistics.Stop()

	if !s.checkBrokerIsAvailable() {
		s.T().Skipf("Broker not available at %s", s.brokerAddress)
		return
	}
}

func (s *AnalyticsSuite) TestBrokerNotEnabled() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var results [][]byte
	go func() {
		defer wg.Done()
		results = s.getMessagesDispatchedByBroker(5 * time.Second)
	}()

	Statistics.Start(&configuration.AnalyticsDefinition{
		Enabled:        false,
		BrokerAddress:  s.brokerAddress,
		BrokerPort:     s.brokerPort,
		BrokerProtocol: s.brokerProto,
		BrokerTopic:    s.brokerTopic,
	})
	assert.Nil(s.T(), Statistics.brokerClient)

	for i := 0; i < 5; i++ {
		Statistics.sendEvent(float64(i)*100.0, fmt.Sprintf("testkey[%d]", i))
		<-time.After(time.Duration(rand.Intn(300)) * time.Millisecond)
	}

	// let the buffer clear
	<-time.After(2 * time.Second)

	Statistics.Stop()
	assert.Nil(s.T(), Statistics.brokerClient)

	wg.Wait()
	assert.Len(s.T(), results, 0)
}

func (s *AnalyticsSuite) TestBrokerCannotConnect() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var results [][]byte
	go func() {
		defer wg.Done()
		results = s.getMessagesDispatchedByBroker(5 * time.Second)
	}()

	Statistics.Start(&configuration.AnalyticsDefinition{
		Enabled:        true,
		BrokerAddress:  "9.9.9.9",
		BrokerPort:     s.brokerPort,
		BrokerProtocol: s.brokerProto,
		BrokerTopic:    s.brokerTopic,
	})
	assert.NotNil(s.T(), Statistics.brokerClient)

	for i := 0; i < 5; i++ {
		Statistics.sendEvent(float64(i)*100.0, fmt.Sprintf("testkey[%d]", i))
		<-time.After(time.Duration(rand.Intn(300)) * time.Millisecond)
	}

	// let the buffer clear
	<-time.After(2 * time.Second)

	Statistics.Stop()
	assert.Nil(s.T(), Statistics.brokerClient)

	wg.Wait()
	assert.Len(s.T(), results, 0)
}

func (s *AnalyticsSuite) TestBrokerSendEvents() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var results [][]byte
	go func() {
		defer wg.Done()
		results = s.getMessagesDispatchedByBroker(5 * time.Second)
	}()

	Statistics.Start(&configuration.AnalyticsDefinition{
		Enabled:        true,
		BrokerAddress:  s.brokerAddress,
		BrokerPort:     s.brokerPort,
		BrokerProtocol: s.brokerProto,
		BrokerTopic:    s.brokerTopic,
	})
	assert.NotNil(s.T(), Statistics.brokerClient)

	for i := 0; i < 5; i++ {
		Statistics.sendEvent(float64(i)*100.0, fmt.Sprintf("testkey[%d]", i))
		<-time.After(time.Duration(rand.Intn(300)) * time.Millisecond)
	}

	// let the buffer clear
	<-time.After(2 * time.Second)

	Statistics.Stop()
	assert.Nil(s.T(), Statistics.brokerClient)

	wg.Wait()
	assert.Len(s.T(), results, 5)

	for i := 0; i < 5; i++ {
		var info AnalyticsEvent

		assert.Nil(s.T(), json.Unmarshal(results[i], &info))

		assert.Equal(s.T(), fmt.Sprintf("testkey[%d]", i), info.ID)
		assert.Equal(s.T(), float64(i)*100.0, info.Value)
		assert.NotEqual(s.T(), time.Time{}, info.Timestamp)
	}
}

func (s *AnalyticsSuite) TestBrokerSendEventsFromDataRoutines() {
	wg := &sync.WaitGroup{}
	wg.Add(3)
	var results [][]byte
	go func() {
		defer wg.Done()
		results = s.getMessagesDispatchedByBroker(5 * time.Second)
	}()

	Statistics.Start(&configuration.AnalyticsDefinition{
		Enabled:        true,
		BrokerAddress:  s.brokerAddress,
		BrokerPort:     s.brokerPort,
		BrokerProtocol: s.brokerProto,
		BrokerTopic:    s.brokerTopic,
	})
	assert.NotNil(s.T(), Statistics.brokerClient)

	go func() {
		defer wg.Done()
		for i := 1; i <= 10; i++ {
			Statistics.SetCounter(float64(i)*100.0, "event_counter", fmt.Sprintf("%d", i))
			Statistics.IncrementCounter(float64(i)*50.0, "event_counter", fmt.Sprintf("%d", i))
			Statistics.DecrementCounter(float64(i)*20.0, "event_counter", fmt.Sprintf("%d", i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 1; i <= 10; i++ {
			Statistics.SetCounter(float64(i)*200.0, "other_counter", fmt.Sprintf("%d", i))
			Statistics.IncrementCounter(float64(i)*50.0, "other_counter", fmt.Sprintf("%d", i))
			Statistics.DecrementCounter(float64(i)*20.0, "other_counter", fmt.Sprintf("%d", i))
		}
	}()

	// let the buffer clear
	<-time.After(2 * time.Second)

	Statistics.Stop()
	assert.Nil(s.T(), Statistics.brokerClient)

	wg.Wait()
	assert.Len(s.T(), results, 60)

	var event1counter = 0
	var event2counter = 0
	for i := 0; i < 60; i++ {
		var info AnalyticsEvent

		assert.Nil(s.T(), json.Unmarshal(results[i], &info))

		if strings.Contains(info.ID, "event_counter") {
			event1counter++
		} else if strings.Contains(info.ID, "other_counter") {
			event2counter++
		} else {
			s.FailNow("unexpected event received")
		}
		assert.NotEqual(s.T(), 0.0, info.Value)
		assert.NotEqual(s.T(), time.Time{}, info.Timestamp)
	}

	assert.Equal(s.T(), 30, event1counter)
	assert.Equal(s.T(), 30, event2counter)
}

// --- Utils --- //
func (s *AnalyticsSuite) checkBrokerIsAvailable() bool {
	options := mqtt.NewClientOptions()
	host, _ := os.Hostname()
	options.ClientID = "test-" + version.Version() + "-" + host + "-" + time.Now().Format(time.RFC3339)
	options.ProtocolVersion = 4 // 3.1.1
	options.AutoReconnect = true
	options.Order = true
	options.KeepAlive = 60
	options.PingTimeout = 1 * time.Second
	options.ConnectTimeout = 2 * time.Second

	options.SetDefaultPublishHandler(outMsgHandler)
	options.Store = mqtt.NewMemoryStore()
	options.AddBroker(fmt.Sprintf("%s://%s:%d", s.brokerProto, s.brokerAddress, s.brokerPort))

	brokerClient := mqtt.NewClient(options)
	token := brokerClient.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not connect broker: %v", token.Error())
		return false
	}

	token = brokerClient.Subscribe(s.brokerTopic, 0, nil)
	if !token.WaitTimeout(1*time.Second) || token.Error() != nil {
		logger.Error("Could not subscribe to topic on broker: %v", token.Error())
		return false
	}

	token = brokerClient.Unsubscribe(s.brokerTopic)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not unsubscribe from topic on broker: %v", token.Error())
		return false
	}
	brokerClient.Disconnect(250)

	return true
}

func (s *AnalyticsSuite) getMessagesDispatchedByBroker(timeout time.Duration) [][]byte {
	options := mqtt.NewClientOptions()
	host, _ := os.Hostname()
	options.ClientID = "test-" + version.Version() + "-" + host + "-" + time.Now().Format(time.RFC3339)
	options.ProtocolVersion = 4 // 3.1.1
	options.AutoReconnect = true
	options.Order = true
	options.KeepAlive = 60
	options.PingTimeout = 1 * time.Second
	options.ConnectTimeout = 2 * time.Second

	options.SetDefaultPublishHandler(outMsgHandler)
	options.Store = mqtt.NewMemoryStore()
	options.AddBroker(fmt.Sprintf("%s://%s:%d", s.brokerProto, s.brokerAddress, s.brokerPort))

	brokerClient := mqtt.NewClient(options)
	token := brokerClient.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not connect broker: %v", token.Error())
		return nil
	}

	var receivedPayloads = [][]byte{}
	var mt = &sync.Mutex{}
	token = brokerClient.Subscribe(s.brokerTopic, 0, func(_ mqtt.Client, message mqtt.Message) {
		mt.Lock()
		defer mt.Unlock()
		logger.Info("DATA: %v", string(message.Payload()))
		receivedPayloads = append(receivedPayloads, message.Payload())
	})
	if !token.WaitTimeout(1*time.Second) || token.Error() != nil {
		logger.Error("Could not subscribe to topic on broker: %v", token.Error())
	}

	<-time.After(timeout)

	token = brokerClient.Unsubscribe(s.brokerTopic)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not unsubscribe from topic on broker: %v", token.Error())
	}
	brokerClient.Disconnect(250)

	return receivedPayloads
}
