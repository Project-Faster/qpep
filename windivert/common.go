package windivert

const (
	// DIVERT_OK No error from diverter library
	DIVERT_OK = 0
	// DIVERT_ERROR_NOTINITILIZED Could not initialize diverter library
	DIVERT_ERROR_NOTINITILIZED = 1
	// DIVERT_ERROR_ALREADY_INIT Cannot re-initialize diverter library
	DIVERT_ERROR_ALREADY_INIT = 2
	// DIVERT_ERROR_FAILED Failure in diverter library
	DIVERT_ERROR_FAILED = 3
	// DIVERT_ERROR_NOT_OPEN Requested state is not available
	DIVERT_ERROR_NOT_OPEN = 4
)
