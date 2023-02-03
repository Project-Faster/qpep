package api

import "github.com/julienschmidt/httprouter"

// APIRouter struct that encapsulates the registered paths to be served
type APIRouter struct {
	// handler Httprouter that handles the requests to apis and static files
	handler *httprouter.Router
	// clientMode If true the prefixe of the apis is "/client" instead of "/server", also in server mode static files are not served
	clientMode bool
}

// EchoResponse models the response to the "/echo" api and allows to check the vitality of connections to server
type EchoResponse struct {
	// Address public address of the client as seen by the server
	Address string `json:"address"`
	// Port source port of the client
	Port int64 `json:"port"`
	// ServerVersion Version string of the connected server
	ServerVersion string `json:"serverversion"`
}

// VersionsResponse models the response to "/versions" api
type VersionsResponse struct {
	// Server version string of the server
	Server string `json:"server"`
	// Client version string of the client
	Client string `json:"client"`
}

// StatusResponse models the current status of the server
type StatusResponse struct {
	// LastCheck time string formatted via layout RFC3339Nano
	LastCheck string `json:"lastcheck"`
	// ConnectionCounter positive integer counter of the current connection from the client
	ConnectionCounter int `json:"connection_counter"`

	// Potentially more fields common to all will be added here (instead of echo)
}

// StatsInfo models a single statistics info tracked by the server/client
type StatsInfo struct {
	// ID Unique value in the attributes list
	ID int `json:"id"`
	// Attribute Internal name of the attribute
	Attribute string `json:"attribute"`
	// Value string value of the attribute value
	Value string `json:"value"`
	// Name Printable name of the
	Name string `json:"name"`
}

// StatsInfoResponse models the list of statistics being served to an api client
type StatsInfoResponse struct {
	// Data List of the collected statistics for a certain api client
	Data []StatsInfo `json:"data"`
}

// --- Router --- //
// notFoundHandler struct to handle the scenario of an api / file not found
type notFoundHandler struct{}

// notFoundHandler struct to handle the scenario of an api with a method not expected
type methodsNotAllowedHandler struct{}
