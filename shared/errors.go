package shared

import "errors"

var (
	ErrFailed                        = errors.New("failed")
	ErrInvalidBackendOperation       = errors.New("operation not supported by the backend selected")
	ErrFailedGatewayDetect           = errors.New("failed to detect the gateway")
	ErrFailedGatewayConnect          = errors.New("failed to connect to the gateway")
	ErrNoCommand                     = errors.New("could not create command")
	ErrCommandNotStarted             = errors.New("could not start command")
	ErrImpossibleValidationRequested = errors.New("validation is impossible as requested")
	ErrConfigurationValidationFailed = errors.New("configuration values did not pass validation")
	ErrInvalidHeader                 = errors.New("qpep header is malformed")
	ErrInvalidHeaderAddressType      = errors.New("qpep header cannot be decoded because it has wrong ip version")
	ErrInvalidHeaderDataLength       = errors.New("qpep header invalid because the data length is not coherent")
	ErrNonProxyableRequest           = errors.New("qpep cannot handle the proxy request")
	ErrProxyCheckRequest             = errors.New("qpep used a fake api request to check the proxy usage")
)
