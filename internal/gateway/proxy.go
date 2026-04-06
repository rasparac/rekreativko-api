package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
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

	for name, cfg := range services {
		target, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, err
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.Out.Host = pr.In.Host
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
			Transport: otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
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
