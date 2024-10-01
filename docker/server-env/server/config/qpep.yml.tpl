
server:
  local_address: ${QPEP_ADDRESS}
  local_port: ${QPEP_PORT}

protocol:
  backend: ${QPEP_BACKEND}
  buffersize: 512
  idletimeout: 30s
  ccalgorithm: ${QPEP_CCA}
  ccslowstart: ${QPEP_SLOWSTART}

security:
  certificate: server_cert.pem
  private_key: server_key.pem

analytics:
  enabled: true
  topic: /qpep
  address: ${QPEP_BROKER}
  port: 1883
  protocol: tcp

general:
  api_port: 444
  max_retries: 20
  diverter_threads: 4
  use_multistream: true
  prefer_proxy: true
  verbose: false
