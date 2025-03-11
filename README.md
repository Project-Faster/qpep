# Status
[![Build](https://github.com/Project-Faster/qpep/actions/workflows/integration.yml/badge.svg?branch=main)](https://github.com/Project-Faster/qpep/actions/workflows/integration.yml)

# Background context
See https://docs.projectfaster.org/use-cases/vpn-over-satellite/vpn-client-software/optimizing-client-software. This repository includes the Windows port of qpep.

# qpep
Working on improving the Go standalone implementation of qpep, improving documentation. Original full repo: https://github.com/ssloxford/qpep

Basic testing:
* Ziply Fiber Gigabit in Seattle pulling 1GB test file from DigitalOcean AMS3: ~1.6MB/s
* Ziply Fiber Gigabit in Seattle pulling 1GB test file via qpep running in DigitalOcean AMS3, grabbing 1GB file from AMS3: 10-25MB/s. Slows down over the course of the download. 

# Build
Following here are instructions for manual building the additional parts on windows platform.

### Main module
For building the qpep package you'll need:
- Go 1.20.x
- (Windows) A C/C++ complier compatible with CGO eg. [MinGW64](https://www.mingw-w64.org/). Specifically, download [this](https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win64/Personal%20Builds/mingw-builds/8.1.0/threads-posix/seh/x86_64-8.1.0-release-posix-seh-rt_v6-rev0.7z), extract the files, and add the "bin" directory to the PATH.
- (Linux) A C/C++ complier compatible with CGO eg. GCC

After setting the go and c compiler in the PATH, be sure to also check that `go env` reports that:
- `CGO_ENABLED=1` 
- `CC=<path to c compiler exe>`
- `CXX=<path to c++ compiler exe>`

After that change to the "backend" directory, add the `GOPATH/bin` path your PATH and execute`go build -o build\qpep.exe` 
in the root directory, this will build the executable.
To run it, first copy the following files to the executable folder (if you are on 64 bit platform):
- x64\WinDivert.dll
- x64\WinDivert64.sys

#### Note about the windows drivers
The .sys file are windows user mode drivers taken from the [WinDivert](https://reqrypt.org/windivert.html) project site, they install automatically when the qpep client runs and are automatically removed when the program exits.

_There is no need to install those manually_ it might mess up the loading of the driver when running qpep.

### Qpep-tray module
This module compiles without additional dependencies so just cd into qpep-tray directory and run:
`go build -ldflags -H=windowsgui`

Note: Be sure to set the environment variable CGO_ENABLED to 0 to build qpep-tray like this:
- Windows: `set CGO_ENABLED=1`
- Linux: `set CGO_ENABLED=0`

The flags `-ldflags -H=windowsgui` allow the binary to run without a visible console in the background.
It should be placed in the same folder as the qpep main executable and will need to be launched with administrative priviledges to allow the qpep client to work properly.

The configuration file is created automatically on first launch under `%APPDATA%\qpeptray\` and is a yaml file with the following defaults:
```
client:
  local_address: 192.168.1.24
  local_port: 9443
  gateway_address: 0.0.0.0
  gateway_port: 1443

protocol:
  backend: quicly-go
  buffersize: 512
  idletimeout: 30s
  ccalgorithm: cubic
  ccslowstart: search

security:
  certificate: server_cert.pem

general:
  api_port: 444
  max_retries: 50
  diverter_threads: 4
  use_multistream: true
  prefer_proxy: true
  verbose: false

```

Information about their meaning and usage can be found running in the [user manual](https://github.com/Project-Faster/qpep/releases/latest).

The file can also be opened directly from the tray icon selecting "*Edit Configuration*", upon change detected to it, the user will be asked if it wants to reload the configuration relaunching the client / server.

The module can also be built on linux and will work the same except for the menu icons which fail to load currently on that platform (by current limitation of the underlying go package).

### Generating the Windows MSI Installer
Additional dependencies are required to build the msi package:
- Windows Visual Studio 2019+ (Community is ok)
- [WiX Toolkit 3.11.2](https://wixtoolset.org/releases/)

The `installer.bat` script is the recommended way to build it as it takes care of preparing the files necessary and building all the packages required.

Supported flags for it's invocation are:
- `--build64` Will prepare the 64bits version of the executables
- `--rebuild` Will only rebuild the installer without building the binaries again

The resulting package will be created under the `build/` subdirectory.

## References in Publications 
QPEP and the corresponding testbed were both designed to encourage academic research into secure and performant satellite communications. We would be thrilled to learn about projects you're working on academically or in industry which build on QPEP's contribution!

If you use QPEP, the dockerized testbed, or something based on it, please cite the conference paper which introduces QPEP:
> Pavur, James, Martin Strohmeier, Vincent Lenders, and Ivan Martinovic. QPEP: An Actionable Approach to Secure and Performant Broadband From Geostationary Orbit. Network and Distributed System Security Symposium (NDSS 2021), February 2021. [https://ora.ox.ac.uk/objects/uuid:e88a351a-1036-445f-b79d-3d953fc32804](https://ora.ox.ac.uk/objects/uuid:e88a351a-1036-445f-b79d-3d953fc32804).

## License
The Clear BSD License

Copyright (c) 2020 James Pavur.

All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted (subject to the limitations in the disclaimer
below) provided that the following conditions are met:

     * Redistributions of source code must retain the above copyright notice,
     this list of conditions and the following disclaimer.

     * Redistributions in binary form must reproduce the above copyright
     notice, this list of conditions and the following disclaimer in the
     documentation and/or other materials provided with the distribution.

     * Neither the name of the copyright holder nor the names of its
     contributors may be used to endorse or promote products derived from this
     software without specific prior written permission.

NO EXPRESS OR IMPLIED LICENSES TO ANY PARTY'S PATENT RIGHTS ARE GRANTED BY
THIS LICENSE. THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND
CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A
PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR
CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR
BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER
IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.

“Commons Clause” License Condition v1.0

The Software is provided to you by the Licensor under the License, as defined below, subject to the following condition.

Without limiting other conditions in the License, the grant of rights under the License will not include, and the License does not grant to you, the right to Sell the Software.

For purposes of the foregoing, “Sell” means practicing any or all of the rights granted to you under the License to provide to third parties, for a fee or other consideration (including without limitation fees for hosting or consulting/ support services related to the Software), a product or service whose value derives, entirely or substantially, from the functionality of the Software. Any license notice or attribution required by the License must also include this Commons Clause License Condition notice.

## Acknowledgments
[OpenSAND](https://opensand.org/content/home.php) and the [Net4Sat](https://www.net4sat.org/content/home.php) project have been instrumental in making it possible to develop realistic networking simulations for satellite systems.

This project would not have been possible without the incredible libraries developed by the Go community. These libraries are linked as submodules in this git repository. We're especially grateful to the [quic-go](https://github.com/lucas-clemente/quic-go) project.
