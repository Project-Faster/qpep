package configuration

import "time"

var DefaultConfig = QPepConfigType{
	Client: &ClientDefinition{
		LocalListeningAddress: "0.0.0.0",
		LocalListenPort:       9443,
		GatewayHost:           "198.18.0.254",
		GatewayPort:           443,
		MultipathAddressList:  make([]MultipathPathConfig, 0),
	},
	Server: &ServerDefinition{
		LocalListeningAddress:    "",
		LocalListenPort:          0,
		ExternalListeningAddress: "",
	},
	Security: &CertDefinition{
		Certificate: "server_cert.pem",
		PrivateKey:  "",
	},
	Protocol: &ProtocolDefinition{
		Backend:         "quic-go",
		BufferSize:      512,
		IdleTimeout:     30 * time.Second,
		CCAlgorithm:     "cubic",
		CCSlowstartAlgo: "search",
	},
	General: &GeneralDefinition{
		MaxConnectionRetries: 50,
		WinDivertThreads:     4,
		MultiStream:          true,
		PreferProxy:          true,
		APIPort:              444,
		Verbose:              false,
	},
	Limits: &LimitsDefinition{
		Incoming:     nil,
		Outgoing:     nil,
		IgnoredPorts: []int{},
	},
	Analytics: &AnalyticsDefinition{
		Enabled:        false,
		BrokerAddress:  "127.0.0.1",
		BrokerPort:     8080,
		BrokerProtocol: "",
		BrokerTopic:    "",
	},
	Debug: &DebugDefinition{
		DumpPackets:  false,
		MaskRedirect: false,
	},
}
