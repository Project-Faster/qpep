package service

import (
	"bou.ke/monkey"
	"context"
	service "github.com/parvit/kardianos-service"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/workers/client"
	"github.com/parvit/qpep/workers/gateway"
	"github.com/parvit/qpep/workers/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestServiceSuite(t *testing.T) {
	var q ServiceSuite
	suite.Run(t, &q)
}

type ServiceSuite struct{ suite.Suite }

func (s *ServiceSuite) SetupSuite() {}

func (s *ServiceSuite) TearDownSuite() {}

func (s *ServiceSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *ServiceSuite) BeforeTest(_, _ string) {
	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)
}

func (s *ServiceSuite) TestServiceMain_Server() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(server.RunServer, func(context.Context, context.CancelFunc) {})
	monkey.Patch(api.RunServer, func(context.Context, context.CancelFunc, bool) {})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})

	os.Args = os.Args[:1]
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
}

func (s *ServiceSuite) TestServiceMain_Client() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(client.RunClient, func(context.Context, context.CancelFunc) {})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})

	os.Args = append(os.Args[:1], "--client")
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
}

func (s *ServiceSuite) TestServiceStart() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(server.RunServer, func(context.Context, context.CancelFunc) {})
	monkey.Patch(api.RunServer, func(context.Context, context.CancelFunc, bool) {})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})

	os.Args = append(os.Args[:1], "--service", "start")
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
}

func (s *ServiceSuite) TestServiceStop_WhenStopped() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(server.RunServer, func(context.Context, context.CancelFunc) {})
	monkey.Patch(api.RunServer, func(context.Context, context.CancelFunc, bool) {})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})

	os.Args = append(os.Args[:1], "--service", "stop")
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
}

func (s *ServiceSuite) TestServiceStop_WhenStarted() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		svc.status = startedSvc
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	var calledServiceInterrupt = false
	monkey.Patch(sendProcessInterrupt, func() {
		calledServiceInterrupt = true
	})
	monkey.Patch(service.Control, func(service.Service, string) error {
		return svc.Stop()
	})

	os.Args = append(os.Args[:1], "--service", "stop")
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
	assert.Equal(s.T(), stoppedSvc, svc.status)
	assert.True(s.T(), calledServiceInterrupt)
}

func (s *ServiceSuite) TestServiceStatus_Unknown() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: service.StatusUnknown,
		}, nil
	})

	os.Args = append(os.Args[:1], "--service", "status")
	code := ServiceMain()

	assert.Equal(s.T(), WIN32_UNKNOWN_CODE, code)
}

func (s *ServiceSuite) TestServiceStatus_Unexpected() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0xFF,
		}, nil
	})

	os.Args = append(os.Args[:1], "--service", "status")
	code := ServiceMain()

	assert.Equal(s.T(), WIN32_UNKNOWN_CODE, code)
}

func (s *ServiceSuite) TestServiceStatus_Running() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: service.StatusRunning,
		}, nil
	})

	os.Args = append(os.Args[:1], "--service", "status")
	code := ServiceMain()

	assert.Equal(s.T(), WIN32_RUNNING_CODE, code)
}

func (s *ServiceSuite) TestServiceStatus_Stopped() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: service.StatusStopped,
		}, nil
	})

	os.Args = append(os.Args[:1], "--service", "status")
	code := ServiceMain()

	assert.Equal(s.T(), WIN32_STOPPED_CODE, code)
}

func (s *ServiceSuite) TestServiceUnknownCommand() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(server.RunServer, func(context.Context, context.CancelFunc) {})
	monkey.Patch(api.RunServer, func(context.Context, context.CancelFunc, bool) {})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})

	os.Args = append(os.Args[:1], "--service", "test")
	code := ServiceMain()

	assert.Equal(s.T(), WIN32_UNKNOWN_CODE, code)
}

func (s *ServiceSuite) TestServiceInstall() {
	var svc *QPepService

	monkey.Patch(setCurrentWorkingDir, func(_ string) bool { return true })
	monkey.Patch(service.New, func(i service.Interface, _ *service.Config) (service.Service, error) {
		svcStarter := i.(*qpepServiceStarter)
		svc = svcStarter.realService
		return &fakeQPepService{
			q:           svc,
			StatusField: 0,
		}, nil
	})
	monkey.Patch(gateway.SetSystemProxy, func(bool) {})
	var calledServicePerm = false
	monkey.Patch(setServiceUserPermissions, func(string) {
		calledServicePerm = true
	})
	var calledDirPerm = false
	monkey.Patch(setInstallDirectoryPermissions, func(string) {
		calledDirPerm = true
	})

	os.Args = append(os.Args[:1], "--service", "install")
	go ServiceMain()

	<-time.After(1 * time.Second)
	svc.cancelFunc()
	<-time.After(1 * time.Second)

	assert.Equal(s.T(), 0, svc.exitValue)
	assert.True(s.T(), calledServicePerm)
	assert.True(s.T(), calledDirPerm)
}

func (s *ServiceSuite) TestRunAsClient() {
	var called = false
	monkey.Patch(client.RunClient, func(context.Context, context.CancelFunc) {
		called = true
	})

	runAsClient(nil, nil)
	<-time.After(1 * time.Second)

	assert.True(s.T(), called)
}

func (s *ServiceSuite) TestRunAsServer() {
	var calledSrv = false
	monkey.Patch(server.RunServer, func(context.Context, context.CancelFunc) {
		calledSrv = true
	})
	var calledApi = false
	monkey.Patch(api.RunServer, func(_ context.Context, _ context.CancelFunc, localMode bool) {
		calledApi = true
	})

	runAsServer(nil, nil)
	<-time.After(1 * time.Second)

	assert.True(s.T(), calledSrv)
	assert.True(s.T(), calledApi)
}

// --- utilities --- //

type fakeQPepService struct {
	q *QPepService

	StatusField service.Status
}

func (f fakeQPepService) Run() error {
	f.q.Main()
	return nil
}

func (f fakeQPepService) Start() error {
	return f.q.Start()
}

func (f fakeQPepService) Stop() error {
	return f.q.Stop()
}

func (f fakeQPepService) Status() (service.Status, error) {
	return f.StatusField, nil
}

func (f fakeQPepService) Platform() string { return runtime.GOOS }

func (f fakeQPepService) Restart() error { return nil }

func (f fakeQPepService) Install() error { return nil }

func (f fakeQPepService) Uninstall() error { return nil }

func (f fakeQPepService) Logger(errs chan<- error) (service.Logger, error) { return nil, nil }

func (f fakeQPepService) SystemLogger(errs chan<- error) (service.Logger, error) { return nil, nil }

func (f fakeQPepService) String() string { return "fakeQPepService" }

var _ service.Service = &fakeQPepService{}
