
FROM opensearchproject/logstash-oss-with-opensearch-output-plugin:8.9.0
USER "root"

WORKDIR /usr/share/logstash/bin

RUN sh -c "apt-get update && apt-get install -y ruby git"
RUN sh -c "git clone https://github.com/parvit/logstash-input-mqtt.git"
RUN sh -c "cd logstash-input-mqtt && git switch upgrade && gem build logstash-input-mqtt.gemspec"
RUN ./logstash-plugin install logstash-input-mqtt/logstash-input-mqtt-0.1.5.gem

CMD /usr/local/bin/docker-entrypoint
