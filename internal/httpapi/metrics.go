package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/CTran10/clearance/internal/metrics"
)

func (r *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if r.config.MetricsEnabled && request.URL.Path == "/metrics" && request.Method == http.MethodGet {
		metrics.Handler().ServeHTTP(response, request)
		return
	}

	recorder := &statusRecorder{ResponseWriter: response}
	started := time.Now()
	defer func() {
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		metrics.ObserveHTTPRequest(request.Method, metricPath(request), strconv.Itoa(status), time.Since(started))
	}()

	r.serveHTTP(recorder, request)
}

func metricPath(request *http.Request) string {
	switch request.URL.Path {
	case "/healthz", "/transactions":
		return request.URL.Path
	}
	if _, ok := transactionIDFromPath(request.URL.Path); ok {
		return "/transactions/{id}"
	}
	if _, ok := depositAccountFromPath(request.URL.Path); ok {
		return "/accounts/{id}/deposits"
	}
	return "/unknown"
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(payload []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(payload)
}
