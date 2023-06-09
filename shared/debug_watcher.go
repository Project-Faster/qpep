package shared

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

const (
	DEBUG_FILE_FMT = "%s_%v_%s.prof"
)

func WatcherCPU() {
	cpuWatcher(0)
}

func cpuWatcher(idx int) {
	t, _ := os.Create(fmt.Sprintf(DEBUG_FILE_FMT, "cpu", idx, time.Now().Format("20060102150405")))
	runtime.SetCPUProfileRate(100)

	pprof.StartCPUProfile(io.Writer(t))

	go func() {
		<-time.After(10 * time.Second)
		pprof.StopCPUProfile()
		t.Sync()
		t.Close()

		go cpuWatcher(idx + 1)
	}()
}

func WatcherHeap() {
	heapWatcher(0)
}

func heapWatcher(idx int) {
	t, _ := os.Create(fmt.Sprintf(DEBUG_FILE_FMT, "heap", idx, time.Now().Format("20060102150405")))
	pprof.WriteHeapProfile(t)
	t.Sync()
	t.Close()

	go func() {
		<-time.After(10 * time.Second)
		heapWatcher(idx + 1)
	}()
}
