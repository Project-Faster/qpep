
client:
  local_address: <QPEP_ADDRESS>
  local_port: 9443
  gateway_address: <QPEP_GATEWAY>
  gateway_port: <QPEP_PORT>
  multipath_address_list:
     - address: <QPEP_ADDRESS>
       port: <QPEP_PORT>

protocol:
  backend: <QPEP_BACKEND>
  buffersize: 512
  idletimeout: 30s
  ccalgorithm: <QPEP_CCA>
  ccslowstart: <QPEP_SLOWSTART>

security:
  certificate: server_cert.pem

general:
  api_port: 444
  max_retries: 20
  diverter_threads: 4
  use_multistream: true
  prefer_proxy: true
  verbose: false
