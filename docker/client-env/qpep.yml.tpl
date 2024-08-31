
# client
gateway: <QPEP_GATEWAY>
port: <QPEP_PORT>
apiport: 444
listenaddress: <QPEP_ADDRESS>
listenport: 9443

# backend
backend: <QPEP_BACKEND>
ccalgorithm: <QPEP_CCA>
ccslowstart: <QPEP_SLOWSTART>
buffersize: 512 # in Kb

# certificate
certificate: server_cert.pem

# default
acks: 10
ackdelay: 25
congestion: 4
decimate: 4
decimatetime: 100
maxretries: 50
multistream: true
verbose: false
preferproxy: true
varackdelay: 0
threads: 4
