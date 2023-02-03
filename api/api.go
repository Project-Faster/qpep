package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"

	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/version"
)

// formatRequest method formats to a string the request in input, if verbose configuration
// is set then also the body of the request is extracted
func formatRequest(r *http.Request) string {
	data, err := httputil.DumpRequest(r, shared.QPepConfig.Verbose)
	if err != nil {
		return fmt.Sprintf("REQUEST: %v", err)
	}

	return string(data)
}

// apiStatus handles the api path /status , which sends as output a json object
// of type StatusResponse
func apiStatus(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	addr := ps.ByName("addr")

	if len(addr) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data, err := json.Marshal(StatusResponse{
		LastCheck:         time.Now().Format(time.RFC3339Nano),
		ConnectionCounter: int(Statistics.GetCounter(PERF_CONN, addr)),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// apiEcho handles the api path /echo , which sends as output a json object
// of type EchoResponse
func apiEcho(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	mappedAddr := r.RemoteAddr

	if !strings.HasPrefix(r.RemoteAddr, "127.") {
		mappedAddr = Statistics.GetMappedAddress(r.RemoteAddr)
		if len(mappedAddr) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	port := int64(0)
	dataAddr := strings.Split(mappedAddr, ":")

	switch len(dataAddr) {
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	case 1:
		break
	case 2:
		port, _ = strconv.ParseInt(dataAddr[1], 10, 64)
	}

	data, err := json.Marshal(EchoResponse{
		Address:       dataAddr[0],
		Port:          port,
		ServerVersion: version.Version(),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	Statistics.SetState(INFO_PLATFORM, r.Header.Get("User-Agent"))
	Statistics.SetState(INFO_UPDATE, time.Now().Format(time.RFC1123Z))
}

// apiVersions handles the api path /versions , which sends as output a json object
// of type VersionsResponse
func apiVersions(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	server := "N/A"
	client := "N/A"
	if strings.Contains(r.URL.String(), API_PREFIX_SERVER) {
		server = version.Version()
	} else {
		client = version.Version()
		server = Statistics.GetState(INFO_OTHER_VERSION)
	}

	data, err := json.Marshal(VersionsResponse{
		Server: server,
		Client: client,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// apiStatisticsHosts handles the api path /statistics/hosts , which responds using a json object
// of type StatsInfoResponse, containing an attribute object StatsInfo of value "Address" for
// every host tracked
func apiStatisticsHosts(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	info := StatsInfoResponse{}
	hosts := Statistics.GetHosts()

	sort.Strings(hosts)
	for i := 0; i < len(hosts); i++ {
		info.Data = append(info.Data, StatsInfo{
			ID:        i + 1,
			Attribute: "Address",
			Value:     hosts[i],
		})
	}

	data, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// apiStatisticsInfo handles the api paths /statistics/info, /statistics/info/:addr, which responds
// using a json object of type StatsInfoResponse, containing attribute objects of type StatsInfo with
// values:
// * INFO_ADDRESS
// * INFO_UPDATE
// * INFO_PLATFORM
// for the requested address (via the _addr_ parameter) or for the responding system if not specified
func apiStatisticsInfo(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	reqAddress := ps.ByName("addr")

	lastUpdate := ""
	address := shared.QPepConfig.ListenHost
	platform := runtime.GOOS
	if len(reqAddress) > 0 {
		address = reqAddress
		platform = Statistics.GetState(INFO_PLATFORM)
		lastUpdate = Statistics.GetState(INFO_UPDATE)
	} else {
		lastUpdate = time.Now().Format(time.RFC1123Z)
	}

	info := StatsInfoResponse{}
	info.Data = make([]StatsInfo, 0, 3)
	info.Data = append(info.Data, StatsInfo{
		ID:        1,
		Attribute: "Address",
		Value:     address,
		Name:      INFO_ADDRESS,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        2,
		Attribute: "Last Update",
		Value:     lastUpdate,
		Name:      INFO_UPDATE,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        3,
		Attribute: "Platform",
		Value:     platform,
		Name:      INFO_PLATFORM,
	})

	data, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// apiStatisticsInfo handles the api paths /statistics/data/:addr, which responds
// using a json object of type StatsInfoResponse, containing attribute objects of type StatsInfo with
// values:
// * PERF_CONN
// * PERF_UP_SPEED
// * PERF_DW_SPEED
// * PERF_UP_TOTAL
// * PERF_DW_TOTAL
// for the requested address or for the responding system if not specified
func apiStatisticsData(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	reqAddress := ps.ByName("addr")

	currConnections := Statistics.GetCounter(PERF_CONN, reqAddress)
	if currConnections == -1.0 {
		currConnections = 0.0
	}

	upSpeed := Statistics.GetCounter(PERF_UP_SPEED, reqAddress)
	dwSpeed := Statistics.GetCounter(PERF_DW_SPEED, reqAddress)
	upTotal := Statistics.GetCounter(PERF_UP_TOTAL, reqAddress)
	dwTotal := Statistics.GetCounter(PERF_DW_TOTAL, reqAddress)
	if upSpeed == -1.0 {
		upSpeed = 0.0
	}
	if dwSpeed == -1.0 {
		dwSpeed = 0.0
	}
	if upTotal == -1.0 {
		upTotal = 0.0
	}
	if dwTotal == -1.0 {
		dwTotal = 0.0
	}

	info := StatsInfoResponse{}
	info.Data = make([]StatsInfo, 0, 5)
	info.Data = append(info.Data, StatsInfo{
		ID:        1,
		Attribute: "Current Connections",
		Value:     strconv.Itoa(int(currConnections)),
		Name:      PERF_CONN,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        2,
		Attribute: "Current Upload Speed",
		Value:     fmt.Sprintf("%.2f", upSpeed),
		Name:      PERF_UP_SPEED,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        3,
		Attribute: "Current Download Speed",
		Value:     fmt.Sprintf("%.2f", dwSpeed),
		Name:      PERF_DW_SPEED,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        4,
		Attribute: "Total Uploaded Bytes",
		Value:     fmt.Sprintf("%.2f", upTotal),
		Name:      PERF_UP_TOTAL,
	})
	info.Data = append(info.Data, StatsInfo{
		ID:        5,
		Attribute: "Total Downloaded Bytes",
		Value:     fmt.Sprintf("%.2f", dwTotal),
		Name:      PERF_DW_TOTAL,
	})

	data, err := json.Marshal(info)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
