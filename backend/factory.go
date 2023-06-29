package backend

import (
	_ "github.com/Project-Faster/quic-go"
	"strings"
)

var bcRegister map[string]QuicBackend
var bcList []string

func Register(key string, backend QuicBackend) {
	if bcRegister == nil {
		bcRegister = make(map[string]QuicBackend)
		bcList = make([]string, 0, 8)
	}
	key = strings.ToLower(key)
	if _, ok := bcRegister[key]; !ok {
		bcRegister[strings.ToLower(key)] = backend
		bcList = append(bcList, key)
		return
	}
}

func Get(key string) (QuicBackend, bool) {
	val, ok := bcRegister[key]
	return val, ok
}

func List() []string {
	return append([]string{}, bcList...)
}
