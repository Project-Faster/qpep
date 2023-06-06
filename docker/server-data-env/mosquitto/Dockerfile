
FROM debian:bullseye
USER "root"

WORKDIR /etc/apt/sources.list.d/

RUN sh -c "apt-get update && apt-get install -y wget gnupg2 net-tools"
RUN sh -c "wget http://repo.mosquitto.org/debian/mosquitto-repo.gpg.key"
RUN sh -c "apt-key add mosquitto-repo.gpg.key"
RUN sh -c "wget https://repo.mosquitto.org/debian/mosquitto-bullseye.list"
RUN sh -c "apt-get update && apt-get install -y mosquitto mosquitto-clients"

WORKDIR /usr/sbin
ENTRYPOINT ["/usr/sbin/mosquitto" , "-v", "-c", "/etc/mosquitto/mosquitto.conf"]
