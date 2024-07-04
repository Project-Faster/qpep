#!/bin/bash -xe

function fail() {
  msg=$1
  echo "$msg"
  echo "***************************"
  echo "**** RESULT: FAILURE   ****"
  echo "***************************"
  exit 1
}

echo "************************************"
echo "**** QPEP OSX INSTALLER BUILDER ****"
echo "************************************"
echo

echo [Prerequisites check: MAC OS]
uname -a | grep Darwin
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Prerequisites check: GO]
go version
if [[ ! "$?" -eq "0" ]]; then
  fail "Not found"
fi

echo [Prerequisites check: CMAKE]
cmake --version
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
rm -rf ./*.dmg
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

cp ../build/qpep QPep.app/Contents/MacOS/
cp ../build/qpep-tray QPep.app/Contents/MacOS/
mkdir -p QPep.app/Contents/MacOS/config

echo [Generate Bundle]
pkgbuild --root "QPep.app" --identifier com.project-faster.qpep --scripts Scripts --install-location "/Applications/QPep.app" DistributionInstall.pkg
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
