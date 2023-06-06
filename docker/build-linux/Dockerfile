
FROM debian:latest
ENV HTTP_PROXY ""
ENV HTTPS_PROXY ""
ENV CGO_ENABLED "0"
ENV QPEP_REPO "https://github.com/Project-Faster/qpep.git"
ENV QPEP_BRANCH "main"
ENV QPEP_REV "HEAD"
ENV GOROOT "/opt/go"
ENV BUILDKIT_PROGRESS "plain"

USER root

RUN sh -c 'mkdir /build'

RUN sh -c 'echo export GOROOT=/opt/go'
RUN sh -c 'echo export PATH=$GOROOT/bin:$PATH'

RUN sh -c 'apt-get update'
RUN sh -c 'apt-get install -y wget git dos2unix iputils-ping'

WORKDIR /opt
RUN sh -c 'wget https://go.dev/dl/go1.18.10.linux-amd64.tar.gz'
RUN sh -c 'tar -xvvf go1.18.10.linux-amd64.tar.gz'

WORKDIR /root
RUN printf '#!/bin/bash -xe \n\
echo [CLONE from ${QPEP_REPO}@${QPEP_BRANCH}:${QPEP_REV}] \n\
cd /root \n\
git clone ${QPEP_REPO} || true     \n\
cd /root/qpep \n\
git switch ${QPEP_BRANCH} \n\
if [[ ! "${QPEP_REV}" -eq "HEAD" ]]; then \n\
  git reset --hard ${QPEP_REV} \n\
fi \n\
export PATH=$GOROOT/bin:$PATH \n\
echo [BUILD] \n\
go build -v -o qpep \n\
chmod +x qpep \n\
cp /root/qpep/qpep /build/qpep \n\
echo [DONE] \n\
' > /root/build-qpep.sh

WORKDIR /root
RUN printf '#!/bin/bash -xe \n\
bash /root/build-qpep.sh \n\
ping -c 3 QPEP_GATEWAY \n\
ping -c 3 QPEP_ADDRESS \n\
ping -c 3 MQTT \n\
cd /build \n\
./qpep -v \n\
' > /root/run-qpep.sh

RUN sh -c 'dos2unix /root/*.sh'
RUN sh -c 'chmod 777 /root/*.sh'
RUN sh -c 'chmod +x /root/*.sh'
