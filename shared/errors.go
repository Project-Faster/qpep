package shared

import "errors"

var (
	ErrFailed                        = errors.New("failed")
	ErrNoCommand                     = errors.New("could not create command")
	ErrCommandNotStarted             = errors.New("could not start command")
	ErrImpossibleValidationRequested = errors.New("validation is impossible as requested")
	ErrConfigurationValidationFailed = errors.New("configuration values did not pass validation")
)
