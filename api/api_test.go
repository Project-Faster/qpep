package api

import (
	"bou.ke/monkey"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAPISuite(t *testing.T) {
	var q APISuite
	suite.Run(t, &q)
}

type APISuite struct {
	suite.Suite
}

func (s *APISuite) BeforeTest(_, testName string) {
}

func (s *APISuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	Statistics.Reset()
	shared.QPepConfig.Verbose = false
}

func (s *APISuite) TestFormatRequest() {
	req, _ := http.NewRequest("POST", "http://localhost:9443", nil)
	assert.NotNil(s.T(), req)

	assert.Equal(s.T(), "POST / HTTP/1.1\r\nHost: localhost:9443\r\n\r\n", formatRequest(req))
}

func (s *APISuite) TestFormatRequest_WithBody() {
	const body = "QPep is a hybrid network accelerator based on the QUIC protocol"
	req, _ := http.NewRequest("POST", "http://localhost:9443", strings.NewReader(body))
	assert.NotNil(s.T(), req)

	shared.QPepConfig.Verbose = true
	assert.Equal(s.T(), "POST / HTTP/1.1\r\nHost: localhost:9443\r\n\r\nQPep is a hybrid network accelerator based on the QUIC protocol", formatRequest(req))
}

func (s *APISuite) TestFormatRequest_Error() {
	req, _ := http.NewRequest("POST", "http://localhost:9443", nil)
	assert.NotNil(s.T(), req)

	monkey.Patch(httputil.DumpRequest, func(*http.Request, bool) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	assert.Equal(s.T(), "REQUEST: test-error", formatRequest(req))
}

func (s *APISuite) TestApiStatus() {
	t := s.T()

	Statistics.SetCounter(15.0, PERF_CONN, "127.0.0.1")

	w := &FakeResponse{}
	apiStatus(w, nil, []httprouter.Param{
		{Key: "addr", Value: "127.0.0.1"},
	})

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.LastCheck) > 0)
	assert.Equal(t, 15, resp.ConnectionCounter)
}

func (s *APISuite) TestApiStatus_NoAddr() {
	t := s.T()

	w := &FakeResponse{}
	apiStatus(w, nil, []httprouter.Param{})

	assert.Equal(t, http.StatusBadRequest, w.status)
}

func (s *APISuite) TestApiStatus_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	apiStatus(w, nil, []httprouter.Param{
		{Key: "addr", Value: "127.0.0.1"},
	})

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

func (s *APISuite) TestApiEcho() {
	t := s.T()

	w := &FakeResponse{}
	apiEcho(w, &http.Request{
		RemoteAddr: "127.0.0.1:8080",
		Header: map[string][]string{
			"User-Agent": {runtime.GOOS},
		},
	}, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp EchoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.Equal(t, "127.0.0.1", resp.Address)
	assert.Equal(t, int64(8080), resp.Port)
	assert.Equal(t, version.Version(), resp.ServerVersion)

	assert.Equal(t, runtime.GOOS, Statistics.GetState(INFO_PLATFORM))
	assert.True(t, len(Statistics.GetState(INFO_UPDATE)) > 0)
}

func (s *APISuite) TestApiEcho_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	apiEcho(w, &http.Request{
		RemoteAddr: "127.0.0.1:8080",
	}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

func (s *APISuite) TestApiEcho_RemoteAddrAlternate() {
	t := s.T()

	w := &FakeResponse{}
	apiEcho(w, &http.Request{
		RemoteAddr: "127.0.0.1",
	}, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp EchoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.Equal(t, "127.0.0.1", resp.Address)
	assert.Equal(t, int64(0), resp.Port)
	assert.Equal(t, version.Version(), resp.ServerVersion)
}

func (s *APISuite) TestApiEcho_RemoteAddrError() {
	t := s.T()

	Statistics.SetMappedAddress("192.168.1.1", "192.168.1.10")

	w := &FakeResponse{}
	apiEcho(w, &http.Request{
		RemoteAddr: "",
	}, nil)

	assert.Equal(t, http.StatusUnauthorized, w.status)
}

func (s *APISuite) TestApiVersions() {
	t := s.T()

	Statistics.SetState(INFO_OTHER_VERSION, version.Version())

	w := &FakeResponse{}
	urlPath, _ := url.Parse("/")
	apiVersions(w, &http.Request{
		URL: urlPath,
	}, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp VersionsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.Equal(t, version.Version(), resp.Server)
	assert.Equal(t, version.Version(), resp.Client)
}

func (s *APISuite) TestApiVersions_ServerSide() {
	t := s.T()

	w := &FakeResponse{}
	urlPath, _ := url.Parse(API_PREFIX_SERVER)
	apiVersions(w, &http.Request{
		URL: urlPath,
	}, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp VersionsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.Equal(t, version.Version(), resp.Server)
	assert.Equal(t, "N/A", resp.Client)
}

func (s *APISuite) TestApiVersions_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	urlPath, _ := url.Parse("/")
	apiVersions(w, &http.Request{
		URL: urlPath,
	}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

func (s *APISuite) TestApiStatisticsHosts() {
	t := s.T()

	Statistics.SetMappedAddress("192.168.1.1", "192.168.1.10")
	Statistics.SetMappedAddress("192.168.1.2", "192.168.1.20")
	Statistics.SetMappedAddress("192.168.1.3", "192.168.1.30")

	w := &FakeResponse{}
	apiStatisticsHosts(w, nil, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 3)

	for id, attr := range resp.Data {
		assert.Equal(t, id+1, attr.ID)
		assert.Equal(t, "Address", attr.Attribute)
		assert.Equal(t, "", attr.Name)

		assert.Equal(t, fmt.Sprintf("192.168.1.%d0", id+1), attr.Value)
	}
}

func (s *APISuite) TestApiStatisticsHosts_EmptyHosts() {
	t := s.T()

	w := &FakeResponse{}
	apiStatisticsHosts(w, nil, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 0)
}

func (s *APISuite) TestApiStatisticsHosts_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	apiStatisticsHosts(w, nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

func (s *APISuite) TestApiStatisticsInfo() {
	t := s.T()

	shared.QPepConfig.ListenHost = "192.168.1.10"

	w := &FakeResponse{}
	apiStatisticsInfo(w, nil, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 3)

	assert.Equal(t, 1, resp.Data[0].ID)
	assert.Equal(t, "Address", resp.Data[0].Attribute)
	assert.Equal(t, shared.QPepConfig.ListenHost, resp.Data[0].Value)
	assert.Equal(t, INFO_ADDRESS, resp.Data[0].Name)

	assert.Equal(t, 2, resp.Data[1].ID)
	assert.Equal(t, "Last Update", resp.Data[1].Attribute)
	assert.True(t, len(resp.Data[1].Value) > 0)
	assert.Equal(t, INFO_UPDATE, resp.Data[1].Name)

	assert.Equal(t, 3, resp.Data[2].ID)
	assert.Equal(t, "Platform", resp.Data[2].Attribute)
	assert.Equal(t, runtime.GOOS, resp.Data[2].Value)
	assert.Equal(t, INFO_PLATFORM, resp.Data[2].Name)
}

func (s *APISuite) TestApiStatisticsInfo_WithAddressParam() {
	t := s.T()

	Statistics.SetState(INFO_PLATFORM, runtime.GOOS)
	Statistics.SetState(INFO_UPDATE, time.Now().Format(time.RFC1123Z))

	w := &FakeResponse{}
	apiStatisticsInfo(w, nil, []httprouter.Param{
		{Key: "addr", Value: "127.0.0.1"},
	})

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 3)

	assert.Equal(t, 1, resp.Data[0].ID)
	assert.Equal(t, "Address", resp.Data[0].Attribute)
	assert.Equal(t, "127.0.0.1", resp.Data[0].Value)
	assert.Equal(t, INFO_ADDRESS, resp.Data[0].Name)

	assert.Equal(t, 2, resp.Data[1].ID)
	assert.Equal(t, "Last Update", resp.Data[1].Attribute)
	assert.True(t, len(resp.Data[1].Value) > 0)
	assert.Equal(t, INFO_UPDATE, resp.Data[1].Name)

	assert.Equal(t, 3, resp.Data[2].ID)
	assert.Equal(t, "Platform", resp.Data[2].Attribute)
	assert.Equal(t, runtime.GOOS, resp.Data[2].Value)
	assert.Equal(t, INFO_PLATFORM, resp.Data[2].Name)
}

func (s *APISuite) TestApiStatisticsInfo_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	apiStatisticsInfo(w, nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

func (s *APISuite) TestApiStatisticsData() {
	t := s.T()

	w := &FakeResponse{}
	apiStatisticsData(w, nil, nil)

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 5)

	assert.Equal(t, 1, resp.Data[0].ID)
	assert.Equal(t, "Current Connections", resp.Data[0].Attribute)
	assert.Equal(t, "0", resp.Data[0].Value)
	assert.Equal(t, PERF_CONN, resp.Data[0].Name)

	assert.Equal(t, 2, resp.Data[1].ID)
	assert.Equal(t, "Current Upload Speed", resp.Data[1].Attribute)
	assert.Equal(t, "0.00", resp.Data[1].Value)
	assert.Equal(t, PERF_UP_SPEED, resp.Data[1].Name)

	assert.Equal(t, 3, resp.Data[2].ID)
	assert.Equal(t, "Current Download Speed", resp.Data[2].Attribute)
	assert.Equal(t, "0.00", resp.Data[2].Value)
	assert.Equal(t, PERF_DW_SPEED, resp.Data[2].Name)

	assert.Equal(t, 4, resp.Data[3].ID)
	assert.Equal(t, "Total Uploaded Bytes", resp.Data[3].Attribute)
	assert.Equal(t, "0.00", resp.Data[3].Value)
	assert.Equal(t, PERF_UP_TOTAL, resp.Data[3].Name)

	assert.Equal(t, 5, resp.Data[4].ID)
	assert.Equal(t, "Total Downloaded Bytes", resp.Data[4].Attribute)
	assert.Equal(t, "0.00", resp.Data[4].Value)
	assert.Equal(t, PERF_DW_TOTAL, resp.Data[4].Name)
}

func (s *APISuite) TestApiStatisticsData_WithAddressParam() {
	t := s.T()

	Statistics.SetCounter(10.0, PERF_CONN, "127.0.0.1")
	Statistics.SetCounter(150.0, PERF_UP_SPEED, "127.0.0.1")
	Statistics.SetCounter(1250.0, PERF_DW_SPEED, "127.0.0.1")
	Statistics.SetCounter(848648.0, PERF_UP_TOTAL, "127.0.0.1")
	Statistics.SetCounter(12848648.0, PERF_DW_TOTAL, "127.0.0.1")

	w := &FakeResponse{}
	apiStatisticsData(w, nil, []httprouter.Param{
		{Key: "addr", Value: "127.0.0.1"},
	})

	assert.Equal(t, http.StatusOK, w.status)
	t.Logf("%v\n", w.Body.String())

	var resp StatsInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Nil(t, err)

	assert.True(t, len(resp.Data) == 5)

	assert.Equal(t, 1, resp.Data[0].ID)
	assert.Equal(t, "Current Connections", resp.Data[0].Attribute)
	assert.Equal(t, "10", resp.Data[0].Value)
	assert.Equal(t, PERF_CONN, resp.Data[0].Name)

	assert.Equal(t, 2, resp.Data[1].ID)
	assert.Equal(t, "Current Upload Speed", resp.Data[1].Attribute)
	assert.Equal(t, "150.00", resp.Data[1].Value)
	assert.Equal(t, PERF_UP_SPEED, resp.Data[1].Name)

	assert.Equal(t, 3, resp.Data[2].ID)
	assert.Equal(t, "Current Download Speed", resp.Data[2].Attribute)
	assert.Equal(t, "1250.00", resp.Data[2].Value)
	assert.Equal(t, PERF_DW_SPEED, resp.Data[2].Name)

	assert.Equal(t, 4, resp.Data[3].ID)
	assert.Equal(t, "Total Uploaded Bytes", resp.Data[3].Attribute)
	assert.Equal(t, "848648.00", resp.Data[3].Value)
	assert.Equal(t, PERF_UP_TOTAL, resp.Data[3].Name)

	assert.Equal(t, 5, resp.Data[4].ID)
	assert.Equal(t, "Total Downloaded Bytes", resp.Data[4].Attribute)
	assert.Equal(t, "12848648.00", resp.Data[4].Value)
	assert.Equal(t, PERF_DW_TOTAL, resp.Data[4].Name)
}

func (s *APISuite) TestApiStatisticsData_FailJSON() {
	t := s.T()

	monkey.Patch(json.Marshal, func(interface{}) ([]byte, error) {
		return nil, errors.New("test-error")
	})

	w := &FakeResponse{}
	apiStatisticsData(w, nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.status)
}

// --- Utils --- //
type FakeResponse struct {
	header http.Header
	status int
	Body   *bytes.Buffer
}

func (r *FakeResponse) Header() http.Header {
	return r.header
}

func (r *FakeResponse) WriteHeader(n int) {
	r.status = n
}

func (r *FakeResponse) Write(b []byte) (n int, err error) {
	if r.Body == nil {
		r.Body = bytes.NewBuffer(make([]byte, 0, 32))
	}
	return r.Body.Write(b)
}
