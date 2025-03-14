
FROM debian:latest

ENV HTTP_PROXY=""
ENV HTTPS_PROXY=""
ENV CGO_ENABLED="1"
ENV GOROOT="/opt/go"
ENV GOPATH="/opt/gopath"
ENV BUILDKIT_PROGRESS="plain"

ARG GO_VERSION
ARG QPEP_BACKEND
ARG QPEP_CCA
ARG QPEP_SLOWSTART
ARG QPEP_ADDRESS
ARG QPEP_GATEWAY
ARG QPEP_REPO
ARG QPEP_BRANCH
ARG QPEP_REV

USER root

RUN sh -c 'mkdir /opt/build'

RUN sh -c 'echo export GOROOT=/opt/go'
RUN sh -c "echo export PATH=$GOROOT/bin:$PATH"

RUN sh -c 'apt-get update'
RUN sh -c 'apt-get install -y wget git dos2unix gettext-base iputils-ping g++'

WORKDIR /opt
RUN sh -c "wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
RUN sh -c "tar -xf go${GO_VERSION}.linux-amd64.tar.gz"

RUN printf "#!/bin/bash -xe \n\
env \n\
echo [CLONE from ${QPEP_REPO}@${QPEP_BRANCH}:${QPEP_REV}] \n\
cd /root \n\
git clone ${QPEP_REPO} || true     \n\
cd /root/qpep \n\
git switch ${QPEP_BRANCH} \n\
if [[ ! \"${QPEP_REV}\" -eq \"HEAD\" ]]; then \n\
  git checkout \"${QPEP_REV}\" \n\
fi \n\
git submodule init \n\
git submodule update \n\
" > /root/clone-qpep.sh

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
echo [CLEANUP] \n\
rm -rf /root/qpep/backends/quicly-go/gen_* \n\
go clean -cache -x -v \n\
' > /root/build-qpep.sh

WORKDIR /root
RUN printf '#!/bin/bash -xe \n\
echo [UPDATE to ${QPEP_REPO}@${QPEP_BRANCH}:${QPEP_REV}] \n\
cd /root/qpep \n\
git switch ${QPEP_BRANCH} \n\
if [[ ! "${QPEP_REV}" -eq "HEAD" ]]; then \n\
  git checkout "${QPEP_REV}" \n\
fi \n\
/root/backends-qpep.sh \n\
/root/build-qpep.sh \n\
' > /root/update-qpep.sh

RUN printf '#!/bin/bash -xe \n\
export QPEP_REPO=$1 \n\
export QPEP_BRANCH=$2 \n\
export QPEP_REV=$3 \n\
export QPEP_CCA=$4 \n\
export QPEP_ADDRESS=$5 \n\
export QPEP_BACKEND=$6 \n\
export QPEP_SLOWSTART=$7 \n\
env \n\
/root/update-qpep.sh \n\
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
