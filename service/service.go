package service

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/parvit/qpep/version"
	"github.com/parvit/qpep/workers/client"
	"github.com/parvit/qpep/workers/server"
	log "github.com/rs/zerolog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	kservice "github.com/parvit/kardianos-service"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/flags"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
)

const (
	// startingSvc indicates that the service is currently starting and is not ready to accept requests
	startingSvc = iota
	// startedSvc indicates that the service can handle incoming requests
	startedSvc
	// stoppingSvc indicates that the service is being stopped and will ignore new requests
	stoppingSvc
	// stoppedSvc indicates a stopped service that does not run and will not handle requests
	stoppedSvc

	// WIN32_RUNNING_CODE Win32 exit code for running status of service
	WIN32_RUNNING_CODE = 0
	// WIN32_STOPPED_CODE Win32 exit code for stopped status of service
	WIN32_STOPPED_CODE = 6
	// WIN32_UNKNOWN_CODE Win32 exit code for not installed status of service
	WIN32_UNKNOWN_CODE = 255

	// serverService server service name
	serverService = "qpep-server"
	// clientService client service name
	clientService = "qpep-client"

	// defaultLinuxWorkDir default working directory for linux platform
	defaultLinuxWorkDir = "/var/run/qpep"
)

type qpepServiceStarter struct {
	realService *QPepService
}

func (p *qpepServiceStarter) Start(_ kservice.Service) error {
	return p.realService.Start()
}
func (p *qpepServiceStarter) Stop(_ kservice.Service) error {
	return p.realService.Stop()
}

// QPepService struct models the service and its internal state to the operating system
type QPepService struct {
	kservice.Service

	// context Termination context
	context context.Context
	// cancelFunc Termination function
	cancelFunc context.CancelFunc
	// status internal running state of the service
	status int
	// exitValue value to be use for exit code
	exitValue int
}

var _ kservice.Service = &QPepService{}

// ServiceMain method wraps the starting logic of the qpep service
func ServiceMain() int {
	flags.ParseFlags(os.Args)

	if flags.Globals.Verbose {
		log.SetGlobalLevel(log.DebugLevel)
	}

	logger.Info("=== QPep version %s ===", version.Version())
	logger.Info(spew.Sdump(flags.Globals))

	execPath, err := os.Executable()
	if err != nil {
		logger.Error("Could not find executable: %s", err)
	}

	workingDir := defaultLinuxWorkDir
	if runtime.GOOS == "windows" {
		workingDir = filepath.Dir(execPath)
		if !setCurrentWorkingDir(workingDir) {
			return 1
		}
	}
	logger.Info("Set workingdir for service child: %s", workingDir)

	ctx, cancel := context.WithCancel(context.Background())
	qpepService := &QPepService{
		context:    ctx,
		cancelFunc: cancel,
	}

	serviceName := serverService
	if flags.Globals.Client {
		serviceName = clientService
	}
	svcConfig := &kservice.Config{
		Name:        serviceName,
		DisplayName: strings.ToTitle(serviceName),
		Description: "QPep - high-latency network accelerator",

		Executable: "qpep.exe",
		Option:     make(kservice.KeyValue),

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

	starter := &qpepServiceStarter{
		realService: qpepService,
	}
	serviceInst, err := kservice.New(starter, svcConfig)
	if err != nil {
		logger.Panic(err.Error())
	}

	svcCommand := flags.Globals.Service

	if len(svcCommand) != 0 {
		// Service control / status run
		if svcCommand == "status" {
			return getStatusCode(serviceInst)
		}

		err = kservice.Control(serviceInst, svcCommand)
		if err != nil {
			logger.Info("Error %v\nPossible actions: %q\n", err.Error(), kservice.ControlAction)
			return WIN32_UNKNOWN_CODE
		}

		if svcCommand == "install" {
			_ = shared.ReadConfiguration(false)
			setServiceUserPermissions(serviceName)
			setInstallDirectoryPermissions(workingDir)
			logger.Info("Service installed correctly")
		}

		logger.Info("Service action %s executed\n", svcCommand)
		return WIN32_RUNNING_CODE
	}

	// As-service run
	logName := "qpep-server.log"
	logLevel := "info"
	if flags.Globals.Client {
		logName = "qpep-client.log"
	}
	if flags.Globals.Verbose {
		logLevel = "debug"
	}
	logger.SetupLogger(logName, logLevel)

	// detect forced interactive mode because service was not installed
	status := getStatusCode(serviceInst)

	if status == WIN32_UNKNOWN_CODE || kservice.ChosenSystem().Interactive() {
		logger.Info("Executes as Interactive mode\n")
		err = qpepService.Main()
	} else {
		logger.Info("Executes as Service mode\n")
		err = serviceInst.Run()
	}

	if err != nil {
		logger.Error("Error while starting QPep service: %v", err)
		qpepService.exitValue = 1 // force error
	}

	logger.Info("Exit errorcode: %d\n", qpepService.exitValue)
	return qpepService.exitValue
}

func getStatusCode(svc kservice.Service) int {
	status, err := svc.Status()
	if err != nil {
		status = kservice.StatusUnknown
	}

	switch status {
	case kservice.StatusRunning:
		return WIN32_RUNNING_CODE

	case kservice.StatusStopped:
		return WIN32_STOPPED_CODE

	default:
		fallthrough
	case kservice.StatusUnknown:
		return WIN32_UNKNOWN_CODE
	}
}

// Start method sets the internal state to startingSvc and then start the Main method.
func (p *QPepService) Start() error {
	logger.Info("Start")

	p.status = startingSvc

	go p.Main()

	return nil // Service is now started
}

// Stop method executes the stopping of the qpep service and sets the status to stoppedSvc
func (p *QPepService) Stop() error {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v\n", err)
		}
		shared.SetSystemProxy(false) // be sure to clear proxy settings on exit
	}()

	logger.Info("Stop")

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

// Main method is called when the service is started and actually initializes all the functionalities
// of the service
func (p *QPepService) Main() error {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v\n", err)
			p.exitValue = 1
		}
		shared.SetSystemProxy(false) // be sure to clear proxy settings on exit
	}()

	logger.Info("Main")

	if err := shared.ReadConfiguration(false); err != nil {
		return err
	}

	if flags.Globals.TraceCPU {
		shared.WatcherCPU()
	}
	if flags.Globals.TraceHeap {
		shared.WatcherHeap()
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

	logger.Info("Shutdown...")
	logger.Info("%d", windivert.CloseWinDivertEngine())

	<-time.After(1 * time.Second)

	logger.Info("Exiting...")
	p.exitValue = 0

	return nil
}

func (p *QPepService) Logger(errs chan<- error) (kservice.Logger, error) {
	return kservice.ConsoleLogger, nil
}

// runAsClient method wraps the logic to setup the system as client mode
func runAsClient(execContext context.Context, cancel context.CancelFunc) {
	logger.Info("Running Client")
	windivert.EnableDiverterLogging(shared.QPepConfig.Verbose)
	go client.RunClient(execContext, cancel)
}

// runAsServer method wraps the logic to setup the system as server mode
func runAsServer(execContext context.Context, cancel context.CancelFunc) {
	logger.Info("Running Server")
	go server.RunServer(execContext, cancel)
	go api.RunServer(execContext, cancel, false)
}
