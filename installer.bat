@echo off

echo ********************************
echo **** QPEP INSTALLER BUILDER ****
echo ********************************
echo.

set /A BUILD64=0
set /A BUILD32=0

if "%1" EQU "--build32" (
    set /A BUILD32=1
)
if "%1" EQU "--build64" (
    set /A BUILD64=1
)
if "%2" EQU "--build32" (
    set /A BUILD32=1
)
if "%2" EQU "--build64" (
    set /A BUILD64=1
)
if "%2" EQU "--rebuild" (
    goto installer
)

if %BUILD64% EQU 0 (
    if %BUILD32% EQU 0 (
        echo Usage: installer.bat [--build32] [--build64]
        echo.
        goto fail
    )
)
if %BUILD32% EQU 0 (
    if %BUILD64% EQU 0 (
        echo Usage: installer.bat [--build32] [--build64]
        echo.
        goto fail
    )
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
RMDIR build\64bits 2> NUL
RMDIR build\32bits 2> NUL
RMDIR build\installer 2> NUL
RMDIR build 2> NUL

echo OK

ECHO [Preparation]
MKDIR build\  2> NUL
if %ERRORLEVEL% GEQ 1 goto fail
MKDIR build\64bits\ 2> NUL
if %ERRORLEVEL% GEQ 1 goto fail
MKDIR build\32bits\ 2> NUL
if %ERRORLEVEL% GEQ 1 goto fail

go clean
if %ERRORLEVEL% GEQ 1 goto fail

COPY /Y windivert\LICENSE build\LICENSE.windivert
if %ERRORLEVEL% GEQ 1 goto fail
COPY /Y LICENSE build\LICENSE
if %ERRORLEVEL% GEQ 1 goto fail

echo OK

set GOOS=windows
if %BUILD64% NEQ 0 (
    set GOARCH=amd64
    go clean -cache

    ECHO [Copy dependencies 64bits]
    COPY /Y windivert\x64\* build\64bits\
    if %ERRORLEVEL% GEQ 1 goto fail
    echo OK

    ECHO [Build 64bits server/client]
    set CGO_ENABLED=1
    go build -o build\64bits\qpep.exe
    if %ERRORLEVEL% GEQ 1 goto fail

    echo OK

    ECHO [Build 64bits tray icon]
    pushd qpep-tray
    if %ERRORLEVEL% GEQ 1 goto fail
    set CGO_ENABLED=0
    go build -ldflags -H=windowsgui -o ..\build\64bits\qpep-tray.exe
    if %ERRORLEVEL% GEQ 1 goto fail
    popd

    echo OK
)

if %BUILD32% NEQ 0 (
    set GOARCH=386
    go clean -cache

    ECHO [Copy dependencies 32bits]
    COPY /Y windivert\x86\* build\32bits\
    if %ERRORLEVEL% GEQ 1 goto fail
    echo OK

    ECHO [Build 32bits server/client]
    set CGO_ENABLED=1
    go build -x -o build\32bits\qpep.exe
    if %ERRORLEVEL% GEQ 1 goto fail

    echo OK

    ECHO [Build 32bits tray icon]
    pushd qpep-tray
    if %ERRORLEVEL% GEQ 1 goto fail
    set CGO_ENABLED=0
    go build -ldflags -H=windowsgui -o ..\build\32bits\qpep-tray.exe
    if %ERRORLEVEL% GEQ 1 goto fail
    popd

    echo OK
)

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

