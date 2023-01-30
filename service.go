package main

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/parvit/qpep/version"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	service "github.com/parvit/kardianos-service"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/client"
	"github.com/parvit/qpep/flags"
	. "github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/server"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
)

const (
	startingSvc = iota
	startedSvc
	stoppingSvc
	stoppedSvc

	WIN32_RUNNING_CODE = 0
	WIN32_STOPPED_CODE = 6
	WIN32_UNKNOWN_CODE = 255

	serverService = "qpep-server"
	clientService = "qpep-client"

	defaultLinuxWorkDir = "/var/run/qpep"
)

type QPepService struct {
	service.Service

	context    context.Context
	cancelFunc context.CancelFunc
	status     int
	exitValue  int
}

func serviceMain() {
	flags.ParseFlags(os.Args)

	execPath, err := os.Executable()
	if err != nil {
		Info("Could not find executable: %s", err)
	}

	workingDir := defaultLinuxWorkDir
	if runtime.GOOS == "windows" {
		workingDir = filepath.Dir(execPath)
		setCurrentWorkingDir(workingDir)
	}
	Info("Set workingdir for service child: %s", workingDir)

	ctx, cancel := context.WithCancel(context.Background())
	svc := QPepService{
		context:    ctx,
		cancelFunc: cancel,
	}

	Info("=== QPep version %s ===", version.Version())
	Info(spew.Sdump(flags.Globals))

	serviceName := serverService
	if flags.Globals.Client {
		serviceName = clientService
	}
	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: strings.ToTitle(serviceName),
		Description: "QPep - high-latency network accelerator",

		Executable: "qpep.exe",
		Option:     make(service.KeyValue),

		WorkingDirectory: workingDir,

		EnvVars:   make(map[string]string),
		Arguments: []string{},
	}

	svcConfig.Option["StartType"] = "manual"
	svcConfig.Option["OnFailure"] = "noaction"

	path, _ := os.LookupEnv("PATH")
	svcConfig.EnvVars["PATH"] = workingDir + ";" + path

	if flags.Globals.Client {
		svcConfig.Arguments = append(svcConfig.Arguments, `--client`)
	}

	serviceInst, err := service.New(&svc, svcConfig)
	if err != nil {
		Info(err.Error())
		panic(err)
	}

	svcCommand := flags.Globals.Service

	if len(svcCommand) != 0 {
		// Service control / status run
		if svcCommand == "status" {
			status, err := serviceInst.Status()
			if err != nil {
				status = service.StatusUnknown
			}

			switch status {
			case service.StatusRunning:
				os.Exit(WIN32_RUNNING_CODE)

			case service.StatusStopped:
				os.Exit(WIN32_STOPPED_CODE)

			default:
				fallthrough
			case service.StatusUnknown:
				os.Exit(WIN32_UNKNOWN_CODE)
			}
		}

		err = service.Control(serviceInst, svcCommand)
		if err != nil {
			Info("Error %v\nPossible actions: %q\n", err.Error(), service.ControlAction)
			os.Exit(WIN32_UNKNOWN_CODE)
			return
		}

		if svcCommand == "install" {
			_ = shared.ReadConfiguration(false)
			setServiceUserPermissions(serviceName)
			setInstallDirectoryPermissions(workingDir)
			Info("Service installed correctly")
		}

		Info("Service action %s executed\n", svcCommand)
		return
	}

	// As-service run
	logName := "qpep-server.log"
	if flags.Globals.Client {
		logName = "qpep-client.log"
	}
	SetupLogger(logName)

	err = serviceInst.Run()
	if err != nil {
		Info("Error while starting QPep service")
	}

	Info("Exit errorcode: %d\n", svc.exitValue)
	os.Exit(svc.exitValue)
}

func (p *QPepService) Start(s service.Service) error {
	Info("Start")

	p.status = startingSvc

	go p.Main()

	return nil // Service is now started
}

func (p *QPepService) Stop(s service.Service) error {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
		}
		shared.SetSystemProxy(false) // be sure to clear proxy settings on exit
	}()

	Info("Stop")

	if p.status != startedSvc {
		p.status = stoppedSvc
		return nil
	}
	p.status = stoppingSvc

	sendProcessInterrupt() // signal the child process to terminate

	execPath, _ := os.Executable()
	name := filepath.Base(execPath)

	waitChildProcessTermination(name) // wait for its actual termination
	p.status = stoppedSvc

	return nil
}

func (p *QPepService) Main() {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
		}
		shared.SetSystemProxy(false) // be sure to clear proxy settings on exit
	}()

	Info("Main")

	if err := shared.ReadConfiguration(false); err != nil {
		panic(err)
	}

	go api.RunServer(p.context, p.cancelFunc, true) // api server for local webgui

	if flags.Globals.Client {
		runAsClient(p.context, p.cancelFunc)
	} else {
		runAsServer(p.context, p.cancelFunc)
	}

	interruptListener := make(chan os.Signal, 1)
	signal.Notify(interruptListener, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

TERMINATIONLOOP:
	for {
		select {
		case <-interruptListener:
			break TERMINATIONLOOP
		case <-p.context.Done():
			break TERMINATIONLOOP
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

	p.cancelFunc()
	<-p.context.Done()

	Info("Shutdown...")
	Info("%d", windivert.CloseWinDivertEngine())

	<-time.After(1 * time.Second)

	Info("Exiting...")
	os.Exit(1)
}

func runAsClient(execContext context.Context, cancel context.CancelFunc) {
	Info("Running Client")

	windivert.EnableDiverterLogging(shared.QPepConfig.Verbose)

	go client.RunClient(execContext, cancel)
}

func runAsServer(execContext context.Context, cancel context.CancelFunc) {
	Info("Running Server")
	go server.RunServer(execContext, cancel)
	go api.RunServer(execContext, cancel, false)
}
