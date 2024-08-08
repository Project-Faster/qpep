package api

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"sync"

	"github.com/julienschmidt/httprouter"
	"github.com/parvit/qpep/flags"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/webgui"
	"github.com/rs/cors"
)

const (
	// API_PREFIX_SERVER string prefix of server apis
	API_PREFIX_SERVER string = "/api/v1/server"
	// API_PREFIX_SERVER string prefix of client apis
	API_PREFIX_CLIENT string = "/api/v1/client"

	// API_ECHO_PATH path of the echo api
	API_ECHO_PATH string = "/echo"
	// API_ECHO_PATH path of the versions api
	API_VERSIONS_PATH string = "/versions"
	// API_ECHO_PATH path of the status api
	API_STATUS_PATH string = "/status/:addr"
	// API_STATS_HOSTS_PATH path of the hosts list api
	API_STATS_HOSTS_PATH string = "/statistics/hosts"
	// API_STATS_INFO_PATH path of the generic statistics info api
	API_STATS_INFO_PATH string = "/statistics/info"
	// API_STATS_DATA_PATH path of the generic statistics data api
	API_STATS_DATA_PATH string = "/statistics/data"
	// API_STATS_INFO_SRV_PATH path of the statistics info api for a certain host
	API_STATS_INFO_SRV_PATH string = "/statistics/info/:addr"
	// API_STATS_DATA_SRV_PATH path of the statistics data api for a certain host
	API_STATS_DATA_SRV_PATH string = "/statistics/data/:addr"
)

// RunServer method initializes a new api server (on an external or local ip), stoppable
// by using the provided context and cancel function
func RunServer(ctx context.Context, cancel context.CancelFunc, localMode bool) {
	// update configuration from flags
	host := shared.QPepConfig.ListenHost
	if localMode {
		host = "127.0.0.1"
		logger.Info("Listening address for local api server set to 127.0.0.1")
	} else {
		host, _ = shared.GetDefaultLanListeningAddress(host, "")
	}
	apiPort := shared.QPepConfig.GatewayAPIPort

	listenAddr := fmt.Sprintf("%s:%d", host, apiPort)
	logger.Info("Opening API Server on: %s", listenAddr)

	rtr := newRouter()
	rtr.clientMode = flags.Globals.Client
	rtr.registerHandlers()
	if localMode {
		rtr.registerStaticFiles()
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	srv := newServer(listenAddr, rtr, ctx)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		if srv != nil {
			srv.Close()
			srv = nil
		}
		cancel()
	}()

	if err := srv.ListenAndServe(); err != nil {
		logger.Info("Error running API server: %v", err)
	}
	srv = nil
	cancel()

	wg.Wait()

	logger.Info("Closed API Server")
}

// newServer method initializes a new api server instance listening on the specified local address
// and with the indicated configured router
func newServer(addr string, rtr *APIRouter, ctx context.Context) *http.Server {
	corsRouterHandler := cors.Default().Handler(rtr.handler)

	return &http.Server{
		Addr:    addr,
		Handler: corsRouterHandler,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}
}

// apiFilter method checks if a request to an api path is really intended to be an api request, checking
// that the "Accept" header be compatible with json content, if not then it returns an http 400 error BadRequest
func apiFilter(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		logger.Info("apiFilter - %s\n", formatRequest(r))

		// Request API request must accept JSON
		accepts := r.Header.Get(textproto.CanonicalMIMEHeaderKey("Accept"))
		if len(accepts) > 0 {
			if !strings.Contains(accepts, "application/json") &&
				!strings.Contains(accepts, "application/*") &&
				!strings.Contains(accepts, "*/*") {

				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		// Request is found for API request
		w.Header().Add("Content-Type", "application/json")
		w.Header().Set("Connection", "close")
		next(w, r, ps)
	})
}

// apiForbidden method returns as response the http status code 403 Forbidden
func apiForbidden(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger.Info("apiForbidden - %s\n", formatRequest(r))

	w.WriteHeader(http.StatusForbidden)
}

// serveFile method returns to the client the file requested (if present in the map), no path traversal is possible
// as all files served are contained in memory and the filesystem is not accessed.
// Makes a best effort to return the correct "Content-Type" to the client in the response.
func serveFile(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger.Info("serveFile - %s\n", formatRequest(r))

	urlPath := r.URL.Path[1:]
	if _, ok := webgui.FilesList[urlPath]; !ok {
		urlPath = "index.html"
	}

	var typeFile string
	if len(filepath.Ext(urlPath)) == 0 {
		typeFile = "text/html"
	} else {
		typeFile = mime.TypeByExtension(urlPath)
	}

	w.Header().Add("Content-Type", typeFile)

	w.Write(webgui.FilesList[urlPath])
}

// --- API router --- //

// newRouter method initializes a new empty http router instance
func newRouter() *APIRouter {
	rtr := httprouter.New()
	rtr.RedirectTrailingSlash = true
	rtr.RedirectFixedPath = true

	return &APIRouter{
		handler: rtr,
	}
}

// registerHandlers method registers all API paths for client and server modes and also installs the default
// handlers to cover scenario where:
// * a request cannot be mapped to a valid path
// * uses an unexpected http method
// * there is an internal exception during the api execution
func (r *APIRouter) registerHandlers() {
	r.handler.PanicHandler = func(w http.ResponseWriter, r *http.Request, i interface{}) {
		logger.Info("PanicHandler - %s\n", formatRequest(r))
		w.WriteHeader(http.StatusInternalServerError)
	}
	r.handler.NotFound = &notFoundHandler{}
	r.handler.MethodNotAllowed = &methodsNotAllowedHandler{}
	r.handler.RedirectTrailingSlash = false
	r.handler.HandleMethodNotAllowed = true

	// register apis with respective allowed usage
	r.registerAPIMethod("GET", API_ECHO_PATH, apiFilter(apiEcho), true, true)
	r.registerAPIMethod("GET", API_VERSIONS_PATH, apiFilter(apiVersions), true, true)
	r.registerAPIMethod("GET", API_STATUS_PATH, apiFilter(apiStatus), true, false)

	r.registerAPIMethod("GET", API_STATS_HOSTS_PATH, apiFilter(apiStatisticsHosts), true, false)
	r.registerAPIMethod("GET", API_STATS_INFO_PATH, apiFilter(apiStatisticsInfo), false, true)
	r.registerAPIMethod("GET", API_STATS_DATA_PATH, apiFilter(apiStatisticsData), true, true)
	r.registerAPIMethod("GET", API_STATS_INFO_SRV_PATH, apiFilter(apiStatisticsInfo), true, false)
	r.registerAPIMethod("GET", API_STATS_DATA_SRV_PATH, apiFilter(apiStatisticsData), true, false)
}

// registerAPIMethod method declares an api path with the provided callback handler, and specifies if the api must be
// reachable when calling from clients, servers or both.
// If an api has a calling mismatch, this will result in an error with status code 403 Forbidden
func (r *APIRouter) registerAPIMethod(method, path string, handle httprouter.Handle, allowServer, allowClient bool) {
	if !allowServer && !allowClient {
		panic(fmt.Sprintf("Requested registration of api method %s %s for neither server or client usage!", method, path))
	}

	logger.Info("Register API: %s %s (srv:%v cli:%v cli-mode:%v)\n", method, path, allowServer, allowClient, r.clientMode)
	if allowServer && !r.clientMode {
		r.handler.Handle(method, API_PREFIX_SERVER+path, handle)
	} else {
		r.handler.Handle(method, API_PREFIX_SERVER+path, apiForbidden)
	}

	if allowClient && r.clientMode {
		r.handler.Handle(method, API_PREFIX_CLIENT+path, handle)
	} else {
		r.handler.Handle(method, API_PREFIX_CLIENT+path, apiForbidden)
	}
}

// registerStaticFiles method installs in the router the non-api routes to obtain the files for the webgui
func (r *APIRouter) registerStaticFiles() {
	for path := range webgui.FilesList {
		if path == "index.html" {
			continue // needs to be handled with a 404 to support the SPA push state router
		}
		r.handler.GET("/"+path, serveFile)
	}
}

// --- Default handlers --- //

// ServeHTTP (notFoundHandler) is invoked in scenarios were the requested path cannot be matched to any installed api path
// returning to the client an error 404, or the index page if the request was not of api type
func (n *notFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Info("notFoundHandler - %s\n", formatRequest(r))

	// Request not found for API request will accept JSON
	accepts := r.Header.Get(textproto.CanonicalMIMEHeaderKey("accept"))
	if len(accepts) > 0 && strings.EqualFold(accepts, "application/json") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Request not found for non-API serves the default page
	serveFile(w, r, nil)
}

// ServeHTTP (methodsNotAllowedHandler) is invoked in scenarios were the requested path was found installed but the
// http method requested was not expected. The response in this case is status code 405 MethodNotAllowed
func (n *methodsNotAllowedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Info("methodsNotAllowedHandler - %s\n", formatRequest(r))
	w.WriteHeader(http.StatusMethodNotAllowed)
}
