package daemon

import (
	"net/http"

	"github.com/gorilla/mux"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/weaveworks/common/middleware"

	transport "github.com/weaveworks/flux/http"
	fluxmetrics "github.com/weaveworks/flux/metrics"
	"github.com/weaveworks/flux/platform"
)

var (
	requestDuration = stdprometheus.NewHistogramVec(stdprometheus.HistogramOpts{
		Namespace: "flux",
		Name:      "request_duration_seconds",
		Help:      "Time (in seconds) spent serving HTTP requests.",
		Buckets:   stdprometheus.DefBuckets,
	}, []string{fluxmetrics.LabelMethod, fluxmetrics.LabelRoute, "status_code", "ws"})
)

// An API server for the daemon
func NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.NewRoute().Name("SyncCluster").Methods("POST").Path("/v1/sync")
	r.NewRoute().Name("SyncStatus").Methods("GET").Path("/v1/sync")

	// We assume every request that doesn't match a route is a client
	// calling an old or hitherto unsupported API.
	r.NewRoute().Name("NotFound").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transport.WriteError(w, r, http.StatusNotFound, transport.MakeAPINotFound(r.URL.Path))
	})

	return r
}

func NewHandler(d *platform.Daemon, r *mux.Router) http.Handler {
	handle := HTTPServer{d}
	r.Get("SyncCluster").HandlerFunc(handle.SyncCluster)
	r.Get("SyncStatus").HandlerFunc(handle.SyncStatus)

	return middleware.Instrument{
		RouteMatcher: r,
		Duration:     requestDuration,
	}.Wrap(r)
}

type HTTPServer struct {
	daemon *platform.Daemon
}

func (s HTTPServer) SyncCluster(w http.ResponseWriter, r *http.Request) {
	err := s.daemon.SyncCluster()
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s HTTPServer) SyncStatus(w http.ResponseWriter, r *http.Request) {
	commits, err := s.daemon.SyncStatus("HEAD")
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	transport.JSONResponse(w, r, commits)
}
