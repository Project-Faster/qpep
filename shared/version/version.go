package version

import (
	"fmt"
)

var strVersion string

var (
	VERSION_MAJOR = 0
	VERSION_MINOR = 7
	VERSION_PATCH = 1
)

func init() {
	strVersion = fmt.Sprintf("%d.%d.%d", VERSION_MAJOR, VERSION_MINOR, VERSION_PATCH)
}

func Version() string {
	return strVersion
}
