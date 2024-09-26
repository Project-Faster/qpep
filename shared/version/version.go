package version

import (
	"fmt"
)

var strVersion string

var (
	VERSION_MAJOR = 0
	VERSION_MINOR = 5
	VERSION_PATCH = 0
)

func init() {
	strVersion = fmt.Sprintf("%d.%d.%d", VERSION_MAJOR, VERSION_MINOR, VERSION_PATCH)
}

func Version() string {
	return strVersion
}
