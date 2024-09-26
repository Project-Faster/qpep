package configuration

import "time"

type ClientDefinition struct {
	// ListenHost (yaml:listenaddress) Address on which the local client listens for incoming connections
	// if subnet is 0. or 127. it will try to autodetect a good ip available
	LocalListeningAddress string `yaml:"local_address"`
	// LocalListenPort (yaml:listenport) Port where qpep will try to redirect the local tcp connections
	LocalListenPort int `yaml:"local_port"`

	// GatewayHost (yaml:gateway_address) Address of gateway qpep server for opening quic connections
	GatewayHost string `yaml:"gateway_address"`
	// GatewayPort (yaml:gateway_port) Port on which the gateway qpep server listens for quic connections
	GatewayPort int `yaml:"gateway_port"`
}

type ServerDefinition struct {
	// ListenHost (yaml:local_address) Address on which the local client listens for incoming connections
	// if subnet is 0. or 127. it will try to autodetect a good ip available
	LocalListeningAddress string `yaml:"local_address"`
	// LocalListenPort (yaml:local_port) Port where qpep will try to redirect the local tcp connections
	LocalListenPort int `yaml:"local_port"`
}

type CertDefinition struct {
	// Certificate (yaml:certificate) Points to the PEM format certificate to use for connections
	Certificate string `yaml:"certificate"`
	// CertKey (yaml:certificate_key) Points to the PEM format private key to use for connections (only required server)
	PrivateKey string `yaml:"private_key"`
}

type ProtocolDefinition struct {
	// Backend (yaml:backend) Specifies the backend to use for quic connections(available: quic-go and quicly-go)
	Backend string `yaml:"backend"`
	// Buffersize (yaml:buffersize) Specifies the size (in Kb) of the buffer used for receiving and sending data in the workers
	BufferSize int `yaml:"buffersize"`
	// IdleTimeout (yaml:idletimeout) Timeout after which, without activity, a connected quic stream is closed
	IdleTimeout time.Duration `yaml:"idletimeout"`
	// CCAlgorithm (yaml:ccalgorithm) String passed to the quic backend to select the congestion algorithm to use
	CCAlgorithm string `yaml:"ccalgorithm"`
	// CCSlowstartAlgo (yaml:ccslowstart) String passed to the quic backend to select the slowstart algorithm for the CCA
	CCSlowstartAlgo string `yaml:"ccslowstart"`
}

type GeneralDefinition struct {
	// MaxConnectionRetries (yaml:max_retries) integer value for maximum number of connection tries to server before
	// stopping (half of these will be with diverter system, the other half with proxy)
	MaxConnectionRetries int `yaml:"max_retries"`
	// WinDivertThreads (yaml:diverter_threads) Indicates the number of threads that the diverter should use to handle packages
	WinDivertThreads int `yaml:"diverter_threads"`
	// MultiStream (yaml:use_multistream) Indicates if the backends should try to use the same connection for the same stream or not
	MultiStream bool `yaml:"use_multistream"`
	// PreferProxy (yaml:prefer_proxy) If true the first half of retries will use the proxy system instead of diverter
	PreferProxy bool `yaml:"prefer_proxy"`
	// APIPort (yaml:api_port) Port on which the gateway qpep server listens for TCP API requests
	APIPort int `yaml:"api_port"`
	// Verbose (yaml:verbose) Activates more verbose output than normal
	Verbose bool `yaml:"verbose"`
}

// LimitsDefinition struct models the map of possible speed limits for incoming and outgoing connections
// an example of a limit definition would be "wikipedia.org: 100K"
// which would limit the speed to 100Kb/s (or a suffix M for 100Mb/s) either for clients connecting
// from wikipedia.org or for connections established to it.
// Domain names shall not have wildcards, addresses can be ranges in the CIDR format
// (eg. 192.168.1.1/24, which indicates 192.168.1.1 - 192.168.1.127)
// Negative speed values will be set to 0, which has the effective function of a blacklist in incoming
// or outgoing direction
type LimitsDefinition struct {
	// Incoming (yaml:clients) key defines the speed limits for incoming connections
	Incoming map[string]string `yaml:"incoming"`
	// Outgoing (yaml:destinations) key defines the speed limits for outgoing connections
	Outgoing map[string]string `yaml:"outgoing"`
	// IgnoredPorts list of network ports to be excluded from redirection
	IgnoredPorts []int `yaml:"ignored_ports"`
}

// AnalyticsDefinition struct models the configuration values for the analytics client, by default it
// remains disabled
type AnalyticsDefinition struct {
	// Enabled (yaml:enabled) allows to quickly enable or disable the analytics configuration
	Enabled bool `yaml:"enabled"`
	// BrokerAddress (yaml:address) Address of broker instance
	BrokerAddress string `yaml:"address"`
	// BrokerPort (yaml:port) Port of broker instance
	BrokerPort int `yaml:"port"`
	// BrokerProtocol (yaml:protocol) Protocol for broker connection
	BrokerProtocol string `yaml:"protocol"`
	// BrokerTopic (yaml:topic) Topic on which to publish the data
	BrokerTopic string `yaml:"topic"`
}

type DebugDefinition struct {
	DumpPackets bool `yaml:"dump_packets"`

	MaskRedirect bool `yaml:"mask_redirect"`
}

func (q *ClientDefinition) merge(r *ClientDefinition) {
	if r != nil {
		q.LocalListeningAddress = r.LocalListeningAddress
		q.LocalListenPort = r.LocalListenPort
		q.GatewayHost = r.GatewayHost
		q.GatewayPort = r.GatewayPort
	}
}

func (q *ServerDefinition) merge(r *ServerDefinition) {
	if r != nil {
		q.LocalListeningAddress = r.LocalListeningAddress
		q.LocalListenPort = r.LocalListenPort
	}
}

func (q *CertDefinition) merge(r *CertDefinition) {
	if r != nil {
		q.Certificate = r.Certificate
		q.PrivateKey = r.PrivateKey
	}
}

func (q *ProtocolDefinition) merge(r *ProtocolDefinition) {
	if r != nil {
		q.Backend = r.Backend
		q.BufferSize = r.BufferSize
		q.IdleTimeout = r.IdleTimeout
		q.CCAlgorithm = r.CCAlgorithm
		q.CCSlowstartAlgo = r.CCSlowstartAlgo
	}
}

func (q *GeneralDefinition) merge(r *GeneralDefinition) {
	if r != nil {
		q.MaxConnectionRetries = r.MaxConnectionRetries
		q.WinDivertThreads = r.WinDivertThreads
		q.MultiStream = r.MultiStream
		q.PreferProxy = r.PreferProxy
		q.APIPort = r.APIPort
		q.Verbose = r.Verbose
	}
}

func (q *LimitsDefinition) merge(r *LimitsDefinition) {
	if r != nil {
		q.Incoming = r.Incoming
		q.Outgoing = r.Outgoing
		q.IgnoredPorts = r.IgnoredPorts
	}
}

func (q *AnalyticsDefinition) merge(r *AnalyticsDefinition) {
	if r != nil {
		q.Enabled = r.Enabled
		q.BrokerAddress = r.BrokerAddress
		q.BrokerPort = r.BrokerPort
		q.BrokerProtocol = r.BrokerProtocol
		q.BrokerTopic = r.BrokerTopic
	}
}

func (q *DebugDefinition) merge(r *DebugDefinition) {
	if r != nil {
		q.DumpPackets = r.DumpPackets
		q.MaskRedirect = r.MaskRedirect
	}
}
