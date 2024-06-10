#!/bin/bash -xe

echo "************************************"
echo "**** QPEP OSX INSTALLER BUILDER ****"
echo "************************************"
echo

echo [Prerequisites check: MAC OS]
uname -a | grep Darwin
if [[ ! "$?" -eq "0" ]]; then
  exit 1
fi

echo [Prerequisites check: GO]
go version
if [[ ! "$?" -eq "0" ]]; then
  exit 1
fi

echo [Prerequisites check: CMAKE]
cmake --version
if [[ ! "$?" -eq "0" ]]; then
  exit 1
fi

echo [Prerequisites check: DMGBUILD]
dmgbuild -h || true
if [[ ! "$?" -eq "0" ]]; then
  exit 1
fi

echo [Cleanup]
cd installer-osx
rm -rf *.dmg
rm -rf QPep.app/Contents/MacOS/*

echo [Copy artifacts]
if [[ ! -d "../build" ]]; then
  echo "Error: No build directory found"
  exit 1
fi
if [[ ! -f "../build/qpep" ]]; then
  echo "Error: No qpep executable found"
  exit 1
fi
if [[ ! -f "../build/qpep-tray" ]]; then
  echo "Error: No qpep-tray executable found"
  exit 1
fi

mv ../build/qpep QPep.app/Contents/MacOS/
mv ../build/qpep-tray QPep.app/Contents/MacOS/

dmgbuild -s mac_settings.py "QPep" qpep.dmg
echo "********************************"
echo "**** RESULT: SUCCESS        ****"
echo "********************************"
exit 0
