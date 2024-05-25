
FROM debian:bookworm
USER "root"

WORKDIR /etc/apt/sources.list.d/

RUN sh -c "apt-get update && apt-get install -y wget gnupg2 net-tools"
RUN sh -c "wget http://repo.mosquitto.org/debian/mosquitto-repo.gpg.key"
RUN sh -c "apt-key add mosquitto-repo.gpg.key"
RUN sh -c "apt-key list"
RUN sh -c "echo \"deb https://repo.mosquitto.org/debian bookworm main\" > /etc/apt/sources.list.d/mosquitto-bookworm.list"
RUN sh -c "apt-get update && apt-get install -y mosquitto mosquitto-clients"

WORKDIR /usr/sbin
ENTRYPOINT ["/usr/sbin/mosquitto" , "-v", "-c", "/etc/mosquitto/mosquitto.conf"]
