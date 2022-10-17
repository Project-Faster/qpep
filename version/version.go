package version

import (
	"fmt"
)

var strVersion string

func init() {
	strVersion = fmt.Sprintf("%d.%d.%d", VERSION_MAJOR, VERSION_MINOR, VERSION_PATCH)
}

func Version() string {
	return strVersion
}
