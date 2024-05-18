
# server
gateway: ${QPEP_GATEWAY}
port: 443
apiport: 444
listenaddress: ${QPEP_ADDRESS}
listenport: ${QPEP_PORT}

# backend
backend: ${QPEP_BACKEND}
ccalgorithm: ${QPEP_CCA}
ccslowstart: ${QPEP_SLOWSTART}

# certificate
certificate: server_cert.pem
certificate_key: server_key.pem

# broker settings
analytics:
  enabled: true
  topic: /qpep
  address: MQTT # added via extra_hosts
  port: 1883
  protocol: tcp

# default
acks: 10
ackdelay: 25
congestion: 4
decimate: 4
decimatetime: 100
maxretries: 100
multistream: true
verbose: false
preferproxy: true
varackdelay: 0
threads: 4
