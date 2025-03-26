@echo off

rem "C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat"

echo ********************************
echo **** QPEP INSTALLER BUILDER ****
echo ********************************
echo.

if "%1" EQU "--rebuild" (
    goto installer
)

ECHO [Requirements check: GO]
go version
if %ERRORLEVEL% GEQ 1 goto fail

ECHO [Requirements check: MSBuild]
msbuild --version
if %ERRORLEVEL% GEQ 1 goto fail

ECHO [Requirements check: Wix Toolkit]
heat -help
if %ERRORLEVEL% GEQ 1 goto fail

ECHO [Cleanup]
DEL /S /Q build 2> NUL
RMDIR build\installer 2> NUL
RMDIR build 2> NUL

echo OK

ECHO [Preparation]
MKDIR build\  2> NUL
if %ERRORLEVEL% GEQ 1 goto fail

go clean
if %ERRORLEVEL% GEQ 1 goto fail

COPY /Y windivert\LICENSE build\LICENSE.windivert
if %ERRORLEVEL% GEQ 1 goto fail
COPY /Y LICENSE build\LICENSE
if %ERRORLEVEL% GEQ 1 goto fail

echo OK

set GOOS=windows
set GOARCH=amd64
go clean -cache

ECHO [Copy dependencies 64bits]
COPY /Y windivert\x64\* build\
if %ERRORLEVEL% GEQ 1 goto fail
echo OK

ECHO [Build server/client]
set CGO_ENABLED=1

go build -o build\qpep.exe
if %ERRORLEVEL% GEQ 1 goto fail

echo OK

ECHO [Build tray icon]
pushd qpep-tray
if %ERRORLEVEL% GEQ 1 goto fail

set CGO_ENABLED=0
go build -ldflags -H=windowsgui -o ..\build\qpep-tray.exe
if %ERRORLEVEL% GEQ 1 goto fail
popd

echo OK

:installer
echo [Build of installer]
msbuild installer\installer.sln
if %ERRORLEVEL% GEQ 1 goto fail

echo ********************************
echo **** RESULT: SUCCESS        ****
echo ********************************
exit /B 0

:fail
echo ********************************
echo **** RESULT: FAILURE        ****
echo ********************************
exit /B 1

