
FROM debian:latest

ARG HTTP_PROXY=""
ARG HTTPS_PROXY=""
ARG CGO_ENABLED="1"
ARG GOROOT="/opt/go"
ARG GOPATH="/opt/gopath"
ARG BUILDKIT_PROGRESS="plain"
ARG QPEP_BACKEND="quic-go"
ARG QPEP_CCA="reno"
ARG QPEP_SLOWSTART="basic"
ARG QPEP_ADDRESS="127.0.0.1"
ARG QPEP_GATEWAY="127.0.0.1"
ARG GO_VERSION="1.20.14"
ARG QPEP_REPO="https://github.com/Project-Faster/qpep.git"
ARG QPEP_BRANCH="main"
ARG QPEP_REV="HEAD"

USER root

RUN sh -c 'mkdir /opt/build'

RUN sh -c 'echo export GOROOT=/opt/go'
RUN sh -c 'echo export PATH=$GOROOT/bin:$PATH'

RUN sh -c 'apt-get update'
RUN sh -c 'apt-get install -y wget git dos2unix gettext-base iputils-ping g++ cmake'

WORKDIR /opt
RUN sh -c 'wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz'
RUN sh -c 'tar -xf go${GO_VERSION}.linux-amd64.tar.gz'

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
git submodule init \n\
git submodule update \n\
' > /root/clone-qpep.sh

RUN printf '#!/bin/bash -xe \n\
echo [UPDATE to $1@$2:$3] \n\
cd /root/qpep \n\
git switch $2 \n\
if [[ ! "$3" -eq "HEAD" ]]; then \n\
  git reset --hard $3 \n\
fi \n\
git pull \n\
' > /root/update-qpep.sh

RUN printf '#!/bin/bash -xe \n\
echo [BUILD BACKENDS] \n\
export PATH=$GOROOT/bin:$GOPATH/bin:$PATH \n\
echo [GEN BACKENDS] \n\
cd /root/qpep/backend/ \n\
go env \n\
go generate \n\
' > /root/backends-qpep.sh

RUN printf '#!/bin/bash -xe \n\
echo [BUILD] \n\
export PATH=$GOROOT/bin:$GOPATH/bin:$PATH \n\
cd /root/qpep \n\
go build -v -o qpep \n\
echo [INSTALL] \n\
cp /root/qpep/qpep /opt/build/qpep \n\
chmod +x /opt/build/qpep \n\
mkdir -p /opt/build/config \n\
' > /root/build-qpep.sh

WORKDIR /root
RUN printf '#!/bin/bash -xe \n\
/root/update-qpep.sh $1 $2 $3 \n\
/root/backends-qpep.sh \n\
/root/build-qpep.sh \n\
envsubst < /root/qpep/docker/server-env/server/config/qpep.yml.tpl > /opt/build/config/qpep.yml \n\
cat /opt/build/config/qpep.yml \n\
cp /root/qpep/docker/server-env/server/*.pem /opt/build/ \n\
ping -c 3 MQTT \n\
cd /opt/build \n\
./qpep \n\
' > /root/run-qpep.sh

RUN sh -c 'dos2unix /root/*.sh'
RUN sh -c 'chmod 777 /root/*.sh'
RUN sh -c 'chmod +x /root/*.sh'

RUN sh -c 'bash -c /root/clone-qpep.sh'
RUN sh -c 'bash -c /root/backends-qpep.sh'
RUN sh -c 'bash -c /root/build-qpep.sh'
