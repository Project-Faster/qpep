package shared

import "errors"

var (
	ErrFailed                        = errors.New("failed")
	ErrFailedGatewayDetect           = errors.New("failed to detect the gateway")
	ErrNoCommand                     = errors.New("could not create command")
	ErrCommandNotStarted             = errors.New("could not start command")
	ErrImpossibleValidationRequested = errors.New("validation is impossible as requested")
	ErrConfigurationValidationFailed = errors.New("configuration values did not pass validation")
	ErrInvalidHeader                 = errors.New("qpep header is malformed")
	ErrInvalidHeaderAddressType      = errors.New("qpep header cannot be decoded because it has wrong ip version")
	ErrInvalidHeaderDataLength       = errors.New("qpep header invalid because the data length is not coherent")
)
