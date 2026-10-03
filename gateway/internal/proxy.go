package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/rasparac/rekreativko-api/shared/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type (
	SerivceProxy struct {
		name    string
		target  *url.URL
		proxy   *httputil.ReverseProxy
		timeout time.Duration
	}

	ReverseProxy struct {
		logger   *logger.Logger
		services map[string]*SerivceProxy
	}

	ServiceConfig struct {
		URL     string
		Timeout time.Duration
	}
)

func NewReverseProxy(
	services map[string]ServiceConfig,
	logger *logger.Logger,
) (*ReverseProxy, error) {
	rp := &ReverseProxy{
		logger:   logger,
		services: make(map[string]*SerivceProxy, len(services)),
	}

	// DisableKeepAlives avoids reusing a pooled connection to a backend that
	// has since restarted (common during local debugging, but equally
	// possible in production during a rolling deploy). A stale pooled
	// connection fails with "connection reset by peer" before the request
	// ever reaches the backend's handler - and unlike GET, a POST/PUT/DELETE
	// on a dead connection is never safely auto-retried by net/http. Always
	// dialing fresh trades a bit of per-request latency for correctness.
	//
	// The dial timeout and ResponseHeaderTimeout bound connecting to the
	// backend and then waiting for its response headers - not the body.
	// Ordinary requests are also bounded as a whole by the per-service
	// context timeout in ProxyToService; streams (StreamToService) have no
	// such timeout, so these are what still fail an unreachable backend
	// (without a dial timeout, the OS TCP connect timeout of ~2 minutes) or
	// one that never answers. One transport per service, since the timeout
	// differs per service.
	for name, cfg := range services {
		target, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, err
		}

		transport := &http.Transport{
			DialContext:           (&net.Dialer{Timeout: cfg.Timeout}).DialContext,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: cfg.Timeout,
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.Out.URL.Path = pr.In.URL.Path
				pr.SetXForwarded()
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				rp.logger.Error(r.Context(), "failed to proxy request", "error", err)
				http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			},
			ModifyResponse: func(resp *http.Response) error {
				resp.Header.Set("X-ProxiedBy", "rekreativko-gateway")
				resp.Header.Set("X-Service-Name", name)
				return nil
			},
			Transport: otelhttp.NewTransport(transport, otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
				return fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
			})),
		}

		rp.services[name] = &SerivceProxy{
			name:    name,
			timeout: cfg.Timeout,
			target:  target,
			proxy:   proxy,
		}

	}

	return rp, nil
}

// ProxyToService proxies an ordinary request: the whole exchange must finish
// within the service's timeout.
func (rp *ReverseProxy) ProxyToService(serviceName string, w http.ResponseWriter, r *http.Request) {
	srv, ok := rp.services[serviceName]
	if !ok {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), srv.timeout)
	defer cancel()

	req := r.WithContext(ctx)

	srv.proxy.ServeHTTP(w, req)
}

// StreamToService proxies a long-lived streaming response (server-sent
// events). It runs until the client or the backend closes the stream: the
// server's WriteTimeout is lifted for this response and there is no
// per-service context timeout. The backend must still send its response
// headers within the service's timeout (Transport.ResponseHeaderTimeout).
// The client hanging up cancels the request context, which cancels the
// backend request too.
func (rp *ReverseProxy) StreamToService(serviceName string, w http.ResponseWriter, r *http.Request) {
	srv, ok := rp.services[serviceName]
	if !ok {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		rp.logger.Error(r.Context(), "response writer cannot stream", "error", err, "service", serviceName)
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	srv.proxy.ServeHTTP(w, r)
}
