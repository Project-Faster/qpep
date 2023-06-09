#!/bin/bash -x

DEVICE=eth0
PORT=443

if [[ ! "$2" == "" ]]; then
  DEVICE="$2"
fi

if [[ "$1" == "-d" ]]; then
  tc qdisc del dev eth0 ingress
  tc qdisc del dev "${DEVICE}" ingress
  tc qdisc del dev ifb0 root
  ifconfig ifb0 down
  modprobe -r ifb

  exit 0
fi

(( DELAY="$1" ))
if (( DELAY <= 0 )); then
   exit 1
fi

# Add a TC ingress queue to your external interface, by default you shouldn't have one
tc qdisc add dev "${DEVICE}" handle ffff: ingress

# make sure ifb module is loaded and bring up the interface (IFB = Intermediate Functional Block device)
modprobe ifb
ifconfig ifb0 up

# redirect all traffic to the ifb so that we can later filter on the traffic that leaves that interface
tc filter add dev "${DEVICE}" parent ffff: protocol all u32 match u32 0 0 action mirred egress redirect dev ifb0

# we need a root, this one uses priority queues which defaults to not modifying any traffic
tc qdisc add dev ifb0 root handle 1: prio
# add a special queue that induces latency
tc qdisc add dev ifb0 parent 1:1 handle 2: netem delay "${DELAY}ms" 50ms distribution normal
# if we find a packet that matches our destination port, send it to the above queue
tc filter add dev ifb0 protocol ip parent 1:0 prio 1 u32 match ip dport "${PORT}" 0xffff flowid 2:1

exit 0
