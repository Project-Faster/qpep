package shared

import (
	"errors"
	"strconv"
	"strings"
)

/**
* Parts of the code similar to the github.com/jackpal/gateway module
* but the command output parse is different to allow extract also the
* interface ID for interface filtering in the divert engine
 */

func getRouteGatewayInterfaces(output []byte) ([]int64, error) {
	return nil, nil
}
