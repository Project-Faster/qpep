package api

import (
	"encoding/json"
	"fmt"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/version"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"golang.org/x/net/context"
	"os"
	"runtime/debug"
	"time"
)

const (
	MQTT_QUEUE_BUFFER_SIZE = 256 * 256
)

type analyticsClient struct {
	// TopicName string name of the topic to send the data to
	TopicName string
	// BrokerAddress address of the broker to contact
	BrokerAddress string
	// BrokerPort port of the broker to contact
	BrokerPort int
	// BrokerProtocol protocol with which to communicate with broker (valid: tcp)
	BrokerProtocol string

	// infoChannel channel on which the client waits for updates
	infoChannel chan AnalyticsEvent
	// brokerClient is the client for connection to the mqtt-based broker service
	brokerClient mqtt.Client
	// ctx context with which to check for stop of the client
	ctx context.Context
	// cancel function to stop the client
	cancel context.CancelFunc
}

func (c *analyticsClient) Start() {
	logger.Error("launching analytics broker")
	mqtt.DEBUG = &analyticsLogger{level: zerolog.DebugLevel}
	mqtt.ERROR = &analyticsLogger{level: zerolog.ErrorLevel}
	mqtt.WARN = &analyticsLogger{level: zerolog.WarnLevel}
	mqtt.CRITICAL = &analyticsLogger{level: zerolog.PanicLevel}

	c.infoChannel = make(chan AnalyticsEvent, MQTT_QUEUE_BUFFER_SIZE)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	options := mqtt.NewClientOptions()
	host, _ := os.Hostname()
	options.ClientID = "qpep-" + version.Version() + "-" + host + "-" + time.Now().Format(time.RFC3339)
	options.ProtocolVersion = 4 // 3.1.1
	options.AutoReconnect = true
	options.Order = true
	options.KeepAlive = 60
	options.PingTimeout = 1 * time.Second
	options.ConnectTimeout = 2 * time.Second

	options.SetDefaultPublishHandler(outMsgHandler)
	options.Store = mqtt.NewMemoryStore()
	options.AddBroker(fmt.Sprintf("%s://%s:%d", c.BrokerProtocol, c.BrokerAddress, c.BrokerPort))

	c.brokerClient = mqtt.NewClient(options)

	go c.handleBroker()
}

func (c *analyticsClient) handleBroker() {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("analytics broker error: %v", err)
			debug.PrintStack()
			<-time.After(10 * time.Second)
			go c.handleBroker()
			return
		}
		c.Stop()
	}()

	client := c.brokerClient
	if client == nil {
		return
	}

	token := client.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not connect broker: %v", token.Error())
		return
	}

	token = client.Subscribe(c.TopicName, 0, nil)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		logger.Error("Could not subscribe to topic on broker: %v", token.Error())
		client.Disconnect(250)
		return
	}

	var eventsBuffer = make([]AnalyticsEvent, 0, MQTT_QUEUE_BUFFER_SIZE)

	var t = time.NewTicker(1 * time.Second)
	defer t.Stop()

	var flush = false
	for {
		flush = false

		select {
		case <-c.ctx.Done():
			c.cancel()
			return
		case value := <-c.infoChannel:
			eventsBuffer = append(eventsBuffer, value)
			break
		case <-t.C:
			flush = true
			break
		}
		if len(eventsBuffer) >= MQTT_QUEUE_BUFFER_SIZE*2 {
			eventsBuffer = eventsBuffer[MQTT_QUEUE_BUFFER_SIZE:]
			logger.Error("Slow or unconnected broker, extra events buffer discarded")
		}
		if flush || len(eventsBuffer) >= MQTT_QUEUE_BUFFER_SIZE {
			if !client.IsConnected() {
				continue
			}
			oldBuffer := eventsBuffer
			eventsBuffer = make([]AnalyticsEvent, 0, MQTT_QUEUE_BUFFER_SIZE)
			go c.publishEventsBuffer(client, oldBuffer)
		}
	}
}

func (c *analyticsClient) Stop() {
	client := c.brokerClient
	if client == nil {
		return
	}

	if client.IsConnected() {
		token := client.Unsubscribe(c.TopicName)
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			logger.Error("Could not unsubscribe from topic on broker: %v", token.Error())
		}
		client.Disconnect(250)
	}

	c.cancel()
	<-c.ctx.Done()

	c.brokerClient = nil
}

func (c *analyticsClient) SendEvent(value float64, key string) {
	if c == nil || c.infoChannel == nil {
		logger.Debug("Event ignored, analytics disabled: %v - %v", key, value)
		return
	}
	c.infoChannel <- AnalyticsEvent{
		Timestamp: time.Now().UTC(),
		ID:        key,
		Value:     value,
	}
}

func (c *analyticsClient) publishEventsBuffer(client mqtt.Client, events []AnalyticsEvent) {
	if client == nil {
		return
	}

	logger.Debug("Sending %d events to topic %s", len(events), c.TopicName)
	defer logger.Debug("Done")
	for i := 0; i < len(events); i++ {
		var data, err = json.Marshal(events[i])
		if err != nil {
			logger.Error("%v", err)
			continue
		}

		client.Publish(c.TopicName, 0, false, data)
	}
}

// launchAnalyticsBrokerClient starts the analytics client
func (s *statistics) launchAnalyticsBrokerClient(brokerConfig *configuration.AnalyticsDefinition) {
	if !brokerConfig.Enabled {
		logger.Error("Broker client startup was disabled, check the configuration")
		return
	}
	if s.brokerClient != nil {
		return
	}

	s.brokerClient = &analyticsClient{
		TopicName:      brokerConfig.BrokerTopic,
		BrokerAddress:  brokerConfig.BrokerAddress,
		BrokerPort:     brokerConfig.BrokerPort,
		BrokerProtocol: brokerConfig.BrokerProtocol,
	}
	s.brokerClient.Start()
}

// stopAnalyticsBrokerClient starts the analytics client
func (s *statistics) stopAnalyticsBrokerClient() {
	if s.brokerClient != nil {
		s.brokerClient.Stop()
	}
}

var outMsgHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	mqtt.DEBUG.Printf("TOPIC: %s", msg.Topic())
	mqtt.DEBUG.Printf("MSG: %s", msg.Payload())
}

// analyticsLogger logger bridge from mqtt to qpep
type analyticsLogger struct {
	level zerolog.Level
}

func (m *analyticsLogger) Println(v ...interface{}) {
	m.Printf("%v", v)
}

func (m *analyticsLogger) Printf(format string, v ...interface{}) {
	switch m.level {
	case zerolog.DebugLevel:
		logger.Debug(format, v...)
		break
	case zerolog.ErrorLevel:
		logger.Error(format, v...)
		break
	case zerolog.PanicLevel:
		logger.Panic(format, v...)
		break
	case zerolog.InfoLevel:
		fallthrough
	default:
		logger.Info(format, v...)
		break
	}
}

var _ mqtt.Logger = &analyticsLogger{}
