#!/bin/bash -xe

echo "**************************************"
echo "*****  QUICLY-GO BINDINGS REGEN  *****"
echo "**************************************"
echo

function assert_errorcode() {
  if [[ ! "$?" -eq "0" ]]; then
    echo "********************************"
    echo "**** RESULT: FAILURE        ****"
    echo "********************************"
    exit 1
  fi
}

BASEDIR=$(dirname "$(realpath $0)")
OSNAME=$(go env GOOS)

echo [Prerequisites check: C_FOR_GO]
c-for-go -h
if [[ ! "$?" -eq "0" ]]; then
  echo [Build C-FOR-GO]
  cd deps/c-for-go

  mkdir deps
  pwd
  cp -rf ../../../../.github/debug/cc_local deps/
  cp -f ../../../../.github/debug/go.mod .

  go install -v
  assert_errorcode
fi

cd $BASEDIR
echo [Regen Errors package]
c-for-go -nostamp -nocgo -debug -ccdefs -ccincl -path "$BASEDIR" -out quiclylib genspec/errors.$OSNAME.yml
assert_errorcode

echo [Regen Quicly Bindings package]
c-for-go -nostamp -debug -ccdefs -ccincl -path "$BASEDIR" -out internal genspec/bindings.$OSNAME.yml
assert_errorcode

echo
echo "***************************"
echo "****  RESULT: SUCCESS  ****"
echo "***************************"
cd $BASEDIR
exit 0
