package speedtests

import (
	"context"
	"flag"
	"fmt"
	"github.com/parvit/qpep/shared"
	log "github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

var targetURL = flag.String("target_url", "", "url to download")
var connections = flag.Int("connections_num", 1, "simultaneous tcp connections to make to the server")
var expectedSize = flag.Int("expect_mb", 10, "size in MBs of the target file")
var debugProxy = flag.String("debug_proxy", "", "url to download")

var testlog log.Logger

func TestSpeedTestsConfigSuite(t *testing.T) {
	t.Log(*targetURL)
	t.Log(*connections)
	t.Log(*expectedSize)

	assert.True(t, *connections > 0)
	assert.True(t, len(*targetURL) > 0)
	assert.True(t, *expectedSize > 0)

	*expectedSize = 1024 * 1024 * (*expectedSize)

	_logFile, err := os.OpenFile("./speedtests.log", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	assert.Nil(t, err)

	testlog = log.New(_logFile).Level(log.DebugLevel).
		With().Timestamp().Logger()

	defer func() {
		_logFile.Close()
	}()

	var q SpeedTestsConfigSuite
	suite.Run(t, &q)
}

type SpeedTestsConfigSuite struct {
	suite.Suite
}

func (s *SpeedTestsConfigSuite) BeforeTest(suiteName, testName string) {
	testlog.Info().Msgf("Starting test [%s.%s]\n", suiteName, testName)
}
func (s *SpeedTestsConfigSuite) AfterTest(suiteName, testName string) {
	testlog.Info().Msgf("Finished test [%s.%s]\n", suiteName, testName)
}

func idlingTimeout(body io.ReadCloser, cancel context.CancelFunc, activityFlag, toRead *int64, timeout time.Duration) {
	if activityFlag == nil || toRead == nil {
		return
	}
	<-time.After(timeout)
	testlog.Info().Msgf(">> Idle state check, last activity: %v", time.Unix(*activityFlag, 0))
	if time.Now().Unix()-*activityFlag < int64(timeout.Truncate(time.Second).Seconds()) {
		go idlingTimeout(body, cancel, activityFlag, toRead, timeout)
		return
	}
	if *toRead == 0 {
		return
	}
	cancel()
	body.Close()
	testlog.Info().Msgf(">> Cancel for idle state")
}

func (s *SpeedTestsConfigSuite) TestRun() {
	shared.GetSystemProxyEnabled()

	wg := &sync.WaitGroup{}
	wg.Add(*connections)

	f, err := os.Create("output.csv")
	assert.Nil(s.T(), err)
	defer func() {
		_ = f.Sync()
		_ = f.Close()
	}()

	lock := &sync.Mutex{}

	f.WriteString("timestamp,event,value\n")

	for index := 0; index < *connections; index++ {
		go func(id int) {
			var events = make([]string, 0, 256)
			var flagActivity = time.Now().Unix()

			testlog.Info().Msgf("Starting executor #%d\n", id)
			defer func() {
				testlog.Info().Msgf("#%d GET request done, dumping to CSV...", id)

				// dump the captured events to csv
				lock.Lock()
				defer lock.Unlock()
				for _, ev := range events {
					f.WriteString(ev)
				}
				testlog.Info().Msgf("#%d done", id)

				testlog.Info().Msgf("Stopped executor #%d\n", id)
				wg.Done()
			}()

			client, idleTimeout := getClientForAPI(nil)
			assert.NotNil(s.T(), client)
			assert.NotNil(s.T(), targetURL)

			testlog.Info().Msgf("GET request #%d", id)
			resp, err := client.Get(*targetURL)
			assert.Nil(s.T(), err)
			if err != nil {
				testlog.Info().Msgf("GET request failed #%d", id)
				return
			}
			defer resp.Body.Close()

			toRead := resp.ContentLength
			if toRead != int64(*expectedSize) {
				assert.Failf(s.T(), "No response / wrong response", "%d != %d", toRead, int64(*expectedSize))
				return
			}
			var eventTag = fmt.Sprintf("conn-%d-speed", id)

			defer func() {
				if toRead > 0 {
					start := time.Now()
					events = append(events, fmt.Sprintf("%s,%s,%d\n", start.Format(time.RFC3339Nano), eventTag, toRead/1024))
					testlog.Info().Msgf("#%d bytes to read: %d", id, toRead)
				}
				assert.Equalf(s.T(), int64(0), toRead, "Download was incomplete, remaining: %d", toRead)
			}()

			var totalBytesInTimeDelta int64 = 0
			var start = time.Now()
			var buff = make([]byte, 4096)

			ctx, cancel := context.WithCancel(context.Background())
			go idlingTimeout(resp.Body, cancel, &flagActivity, &toRead, idleTimeout)

		READLOOP:
			for toRead > 0 {
				select {
				case <-ctx.Done():
					break READLOOP
				default:
				}

				rd := io.LimitReader(resp.Body, 4096)
				read, err := rd.Read(buff)
				if err != nil && err != io.EOF {
					if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
						<-time.After(1 * time.Millisecond)
						continue
					}
					testlog.Info().Msgf("err: %v", err)
					assert.Failf(s.T(), "failed", "%v", err)
					return
				}
				if read == 0 {
					<-time.After(1 * time.Millisecond)
					continue
				}

				totalBytesInTimeDelta += int64(read)
				toRead -= int64(read)
				flagActivity = time.Now().Unix()

				testlog.Info().Msgf("#%d read: %d, total: %d, toRead: %d", id, read, resp.ContentLength, toRead)
				if time.Since(start) > 1*time.Second {
					start = time.Now()
					//testlog.Info().Msgf("#%d bytes to read: %d", id, toRead)
					events = append(events, fmt.Sprintf("%s,%s,%d\n", start.Format(time.RFC3339Nano), eventTag, totalBytesInTimeDelta/1024))
					totalBytesInTimeDelta = 0
				}
			}
		}(index)
	}

	wg.Wait()
}

func getClientForAPI(localAddr net.Addr) (*http.Client, time.Duration) {
	dialer := &net.Dialer{
		LocalAddr: localAddr,
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	return &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				if *debugProxy != "" {
					return url.Parse(*debugProxy)
				}
				shared.UsingProxy, shared.ProxyAddress = shared.GetSystemProxyEnabled()
				if shared.UsingProxy {
					return shared.ProxyAddress, nil
				}
				return nil, nil
			},
			DialContext:     dialer.DialContext,
			MaxIdleConns:    0,
			IdleConnTimeout: 5 * time.Second,
			//TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}, 5 * time.Second
}
