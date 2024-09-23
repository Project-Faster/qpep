package version

import (
	"fmt"
)

var strVersion string

var (
	VERSION_MAJOR = 0
	VERSION_MINOR = 4
	VERSION_PATCH = 2
)

func init() {
	strVersion = fmt.Sprintf("%d.%d.%d", VERSION_MAJOR, VERSION_MINOR, VERSION_PATCH)
}

func Version() string {
	return strVersion
}
