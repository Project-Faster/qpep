package shared

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/jackpal/gateway"
)

type QuicConfig struct {
	AckElicitingPacketsBeforeAck   int
	AckDecimationDenominator       int
	InitialCongestionWindowPackets int
	MultiStream                    bool
	VarAckDelay                    float64
	MaxAckDelay                    int //in miliseconds, used to determine if decimating
	MinReceivedBeforeAckDecimation int
	ClientFlag                     bool
	MaxConnectionRetries           int
	GatewayIP                      string
	GatewayPort                    int
	GatewayAPIPort                 int
	ListenIP                       string
	ListenPort                     int
	PreferProxy                    bool
	WinDivertThreads               int
	Verbose                        bool
}

const (
	DEFAULT_REDIRECT_RETRIES = 15
)

var (
	QuicConfiguration         QuicConfig
	UsingProxy                = false
	ProxyAddress              *url.URL
	defaultListeningAddress   string
	detectedGatewayInterfaces []int64
	detectedGatewayAddresses  []string
)

type QLogWriter struct {
	*bufio.Writer
}

func (mwc *QLogWriter) Close() error {
	// Noop
	return mwc.Writer.Flush()
}

func ParseFlags(args []string) {
	log.Println("ARGS:", strings.Join(args, " "))

	ackElicitingFlag := flag.Int("acks", 10, "Number of acks to bundle")
	ackDecimationFlag := flag.Int("decimate", 4, "Denominator of Ack Decimation Ratio")
	congestionWindowFlag := flag.Int("congestion", 4, "Number of QUIC packets for initial congestion window")
	maxConnectionRetriesFlag := flag.Int("maxretries", DEFAULT_REDIRECT_RETRIES, "Number of connection retries before shutting down (min 1, max 300)")
	multiStreamFlag := flag.Bool("multistream", true, "Enable multiplexed QUIC streams inside a single session")
	maxAckDelayFlag := flag.Int("ackdelay", 25, "Maximum number of miliseconds to hold back an ack for decimation")
	varAckDelayFlag := flag.Float64("varackdelay", 0.25, "Variable number of miliseconds to hold back an ack for decimation, as multiple of RTT")
	minReceivedBeforeAckDecimationFlag := flag.Int("decimatetime", 100, "Minimum number of packets before initiating ack decimation")
	clientFlag := flag.Bool("client", false, "a bool")
	gatewayHostFlag := flag.String("gateway", "198.18.0.254", "IP address of gateway running qpep server")
	gatewayPortFlag := flag.Int("port", 443, "Port of gateway running qpep server")
	gatewayAPIPortFlag := flag.Int("apiport", 444, "IP address of gateway running qpep server")
	listenHostFlag := flag.String("listenaddress", "0.0.0.0", "IP listen address of qpep client")
	listenPortFlag := flag.Int("listenport", 9443, "Listen Port of qpep client")
	preferProxyFlag := flag.Bool("preferproxy", false, "Try first using proxy instead of diverter")
	winDiverterThreads := flag.Int("threads", 1, "Worker threads for windivert engine (min 1, max 8)")
	verbose := flag.Bool("verbose", false, "Outputs data about diverted connections for debug")

	flag.CommandLine.Parse(args[1:])
	if !flag.Parsed() {
		flag.Usage()
		os.Exit(1)
	}

	QuicConfiguration = QuicConfig{
		AckElicitingPacketsBeforeAck:   *ackElicitingFlag,
		AckDecimationDenominator:       *ackDecimationFlag,
		InitialCongestionWindowPackets: *congestionWindowFlag,
		MultiStream:                    *multiStreamFlag,
		MaxAckDelay:                    *maxAckDelayFlag,
		VarAckDelay:                    *varAckDelayFlag,
		MinReceivedBeforeAckDecimation: *minReceivedBeforeAckDecimationFlag,
		ClientFlag:                     *clientFlag,
		MaxConnectionRetries:           *maxConnectionRetriesFlag,
		GatewayIP:                      *gatewayHostFlag,
		GatewayPort:                    *gatewayPortFlag,
		GatewayAPIPort:                 *gatewayAPIPortFlag,
		ListenIP:                       *listenHostFlag,
		ListenPort:                     *listenPortFlag,
		PreferProxy:                    *preferProxyFlag,
		WinDivertThreads:               *winDiverterThreads,
		Verbose:                        *verbose,
	}

	data, _ := json.MarshalIndent(QuicConfiguration, "", " ")
	log.Printf("%v\n", string(data))
}

func init() {
	var err error
	detectedGatewayInterfaces, detectedGatewayAddresses, err = getRouteGatewayInterfaces()

	fmt.Printf("gateway interfaces: %v\n", detectedGatewayInterfaces)

	if err != nil {
		panic(err)
	}
}

func GetDefaultLanListeningAddress(currentAddress, gatewayAddress string) (string, []int64) {
	if len(defaultListeningAddress) > 0 {
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	if !strings.HasPrefix(currentAddress, "0.") && !strings.HasPrefix(currentAddress, "127.") {
		return currentAddress, detectedGatewayInterfaces
	}

	if len(gatewayAddress) == 0 {
		defaultIP, err := gateway.DiscoverInterface()
		if err != nil {
			panic(fmt.Sprintf("PANIC: Could not discover default lan address and the requested one is not suitable, error: %v\n", err))
			return currentAddress, detectedGatewayInterfaces
		}

		defaultListeningAddress = defaultIP.String()
		log.Printf("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}

	log.Printf("WARNING: Detected invalid listening ip address, trying to autodetect the default route...\n")

	searchIdx := -1
	searchLongest := 0

NEXT:
	for i := 0; i < len(detectedGatewayAddresses); i++ {
		for idx := 0; idx < len(gatewayAddress); idx++ {
			if currentAddress[idx] == gatewayAddress[idx] {
				continue
			}
			if idx >= searchLongest {
				searchIdx = i
				searchLongest = idx
				continue NEXT
			}
		}
	}
	if searchIdx != -1 {
		defaultListeningAddress = detectedGatewayAddresses[searchIdx]
		log.Printf("Found default ip address: %s\n", defaultListeningAddress)
		return defaultListeningAddress, detectedGatewayInterfaces
	}
	defaultListeningAddress = detectedGatewayAddresses[0]
	return defaultListeningAddress, detectedGatewayInterfaces
}

func GetLanListeningAddresses() ([]string, []int64) {
	return detectedGatewayAddresses, detectedGatewayInterfaces
}
