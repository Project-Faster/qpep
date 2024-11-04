package service

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/flags"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/version"
	"github.com/Project-Faster/qpep/workers/client"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/Project-Faster/qpep/workers/server"
	log "github.com/rs/zerolog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	kservice "github.com/parvit/kardianos-service"

	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/windivert"
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

	// defaultLinuxWorkDir default working directory for linux platform
	defaultLinuxWorkDir = "/opt/qpep"
	// defaultDarwinWorkDir default working directory for darwin platform
	defaultDarwinWorkDir = "/Applications/QPep.app/Contents/MacOS/"
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

	workingDir := "./"
	switch runtime.GOOS {
	case "darwin":
		workingDir = defaultDarwinWorkDir
	case "linux":
		workingDir = defaultLinuxWorkDir
	case "windows":
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

	serviceName := PLATFORM_SERVICE_SERVER_NAME
	if flags.Globals.Client {
		serviceName = PLATFORM_SERVICE_CLIENT_NAME
	}
	svcConfig := &kservice.Config{
		Name:        serviceName,
		DisplayName: "QPep",
		Description: "QPep - high-latency network accelerator",

		Executable: PLATFORM_EXE_NAME,
		Option:     make(kservice.KeyValue),

		WorkingDirectory: workingDir,
		UserName:         os.Getenv("USER"),

		EnvVars:   make(map[string]string),
		Arguments: []string{},
	}

	svcConfig.Option["StartType"] = "manual"
	svcConfig.Option["OnFailure"] = "noaction"
	if runtime.GOOS == "darwin" {
		svcConfig.Option["UserService"] = true
		svcConfig.Option["KeepAlive"] = false
		svcConfig.Option["RunAtLoad"] = true
		svcConfig.Option["LogDirectory"] = "/tmp"
	}

	path, _ := os.LookupEnv("PATH")
	svcConfig.EnvVars["PATH"] = fmt.Sprintf("%s%c%s", workingDir, PLATFORM_PATHVAR_SEP, path)

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
			_ = configuration.ReadConfiguration(false)
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
		gateway.SetSystemProxy(false) // be sure to clear proxy settings on exit
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
		// be sure to clear proxy and diverter settings on exit
		gateway.SetSystemProxy(false)
		gateway.SetConnectionDiverter(false, "", "", 0, 0, 0, 0, []int{})
	}()

	logger.Info("Main")
	var lastError error
	p.context = context.WithValue(p.context, "lastError", &lastError)

	if err := configuration.ReadConfiguration(false); err != nil {
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

	var wasInterrupted = false

TERMINATIONLOOP:
	for {
		select {
		case <-interruptListener:
			wasInterrupted = true
			break TERMINATIONLOOP
		case <-p.context.Done():
			break TERMINATIONLOOP
		}
	}

	p.cancelFunc()
	<-p.context.Done()

	logger.Info("Exiting...")
	<-time.After(1 * time.Second)

	p.exitValue = 0

	var errPtr = p.context.Value("lastError").(*error)
	logger.Info("Exiting %v (%d / %v)", wasInterrupted, p.exitValue, *errPtr)

	if !wasInterrupted && *errPtr != nil {
		p.exitValue = 1
		return *errPtr
	}

	return nil
}

func (p *QPepService) Logger(errs chan<- error) (kservice.Logger, error) {
	return kservice.ConsoleLogger, nil
}

// runAsClient method wraps the logic to setup the system as client mode
func runAsClient(execContext context.Context, cancel context.CancelFunc) {
	logger.Info("Running Client")
	windivert.EnableDiverterLogging(flags.Globals.Verbose)
	go client.RunClient(execContext, cancel)
}

// runAsServer method wraps the logic to setup the system as server mode
func runAsServer(execContext context.Context, cancel context.CancelFunc) {
	logger.Info("Running Server")
	go server.RunServer(execContext, cancel)
	go api.RunServer(execContext, cancel, false)
}
