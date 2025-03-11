#!/bin/bash -xe

function fail() {
  msg=$1
  echo "$msg"
  echo "***************************"
  echo "**** RESULT: FAILURE   ****"
  echo "***************************"
  exit 1
}

export QPEP_VERSION=$1
echo "************************************"
echo "**** QPEP OSX INSTALLER BUILDER ****"
echo "**** VERSION: ${QPEP_VERSION}"
echo "************************************"
echo

if [[ "${QPEP_VERSION}" -eq "" ]]; then
  fail "Version not specified"
fi

echo [Prerequisites check: PLATFORM MAC]
uname -a | grep Darwin
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Prerequisites check: GO]
go version
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Prerequisites check: DMGBUILD]
dmgbuild -h &> /dev/null || true
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Prerequisites check: PRODUCTBUILD]
productbuild -h &> /dev/null || true
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Cleanup]
cd installer-osx
rm -rf ./*.pkg
rm -rf QPep.app/Contents/MacOS/*
echo "OK"

echo [Copy artifacts]
if [[ ! -d "../build" ]]; then
  fail "Error: No build directory found"
fi
if [[ ! -f "../build/qpep" ]]; then
  fail "Error: No qpep executable found"
fi
if [[ ! -f "../build/qpep-tray" ]]; then
  fail "Error: No qpep-tray executable found"
fi

mkdir -p QPep.app/Contents/MacOS/config
cp ../build/qpep QPep.app/Contents/MacOS/
cp ../build/qpep-tray QPep.app/Contents/MacOS/

mkdir -p Uninstaller.app/Contents/MacOS/

export QPEP_GATEWAY=192.168.1.100
export QPEP_ADDRESS=0.0.0.0
export QPEP_PORT=1443
export QPEP_BACKEND=quic-go
export QPEP_CCA=reno
export QPEP_SLOWSTART=search
envsubst < ./qpep.yml.tpl > QPep.app/Contents/MacOS/config/qpep.yml
cp ./server_cert.pem QPep.app/Contents/MacOS/server_cert.pem

envsubst < ./Info.plist.tpl > QPep.app/Contents/Info.plist

echo [Generate Install Bundle]
pkgbuild --root "QPep.app" --identifier com.project-faster.qpep --scripts Scripts --install-location "/Applications/QPep.app" DistributionInstall.pkg

echo [Generate Uninstall Bundle]
pkgbuild --root "Uninstaller.app" --identifier com.project-faster.qpep --scripts UninstallScripts --install-location "/Applications" DistributionUninstall.pkg

# for reference if needed to recreate the xml
# productbuild --synthesize --package DistributionInstall.pkg DistributionInstall.xml
# productbuild --synthesize --package DistributionUninstall.pkg DistributionUninstall.xml

echo [Generating Installer]
productbuild --distribution DistributionInstall.xml --package-path . --resources "./Resources" QPepInstaller.pkg

echo [Generating Uninstaller]
productbuild --distribution DistributionUninstall.xml --package-path . --resources "./UninstallerResources" QPepUninstall.pkg

echo "***************************"
echo "**** RESULT: SUCCESS   ****"
echo "***************************"
exit 0
