
FROM debian:latest
ENV HTTP_PROXY ""
ENV HTTPS_PROXY ""
ENV CGO_ENABLED "1"
ENV GOROOT "/opt/go"
ENV GOPATH "/opt/gopath"
ENV BUILDKIT_PROGRESS "plain"

ARG QPEP_REPO="https://github.com/Project-Faster/qpep.git"
ARG QPEP_BRANCH="main"
ARG QPEP_REV="HEAD"
ARG QPEP_BACKEND="quic-go"
ARG QPEP_CCA="reno"

USER root

RUN sh -c 'mkdir /build'

RUN sh -c 'echo export GOROOT=/opt/go'
RUN sh -c 'echo export PATH=$GOROOT/bin:$PATH'

RUN sh -c 'apt-get update'
RUN sh -c 'apt-get install -y wget git dos2unix gettext-base iputils-ping g++ cmake'

WORKDIR /opt
RUN sh -c 'wget https://go.dev/dl/go1.20.14.linux-amd64.tar.gz'
RUN sh -c 'tar -xf go1.20.14.linux-amd64.tar.gz'

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
chmod +x qpep \n\
' > /root/build-qpep.sh

RUN printf '#!/bin/bash -xe \n\
echo [INSTALL] \n\
cp /root/qpep/qpep /build/qpep \n\
mkdir -p /build/config \n\
envsubst < /root/qpep/docker/client-env/qpep.yml.tpl > /build/config/qpep.yml \n\
echo [DONE] \n\
' > /root/install-qpep.sh

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

RUN sh -c 'bash -c /root/clone-qpep.sh'
RUN sh -c 'bash -c /root/backends-qpep.sh'
RUN sh -c 'bash -c /root/build-qpep.sh'
RUN sh -c 'bash -c /root/install-qpep.sh'
