package upstream

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/app/pagewriter"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/logger"
)

// ProxyErrorHandler is a function that will be used to render error pages when
// HTTP proxies fail to connect to upstream servers.
type ProxyErrorHandler func(http.ResponseWriter, *http.Request, error)

// NewProxy creates a new multiUpstreamProxy that can serve requests directed to
// multiple upstreams.
func NewProxy(upstreams options.Upstreams, sigData *options.SignatureData, writer pagewriter.Writer) (http.Handler, error) {
	m := &multiUpstreamProxy{
		serveMux: mux.NewRouter(),
	}

	for _, upstream := range upstreams {
		if upstream.Static {
			m.registerStaticResponseHandler(upstream)
			continue
		}

		u, err := url.Parse(upstream.URI)
		if err != nil {
			return nil, fmt.Errorf("error parsing URI for upstream %q: %w", upstream.ID, err)
		}
		switch u.Scheme {
		case fileScheme:
			m.registerFileServer(upstream, u)
		case httpScheme, httpsScheme:
			m.registerHTTPUpstreamProxy(upstream, u, sigData, writer)
		default:
			return nil, fmt.Errorf("unknown scheme for upstream %q: %q", upstream.ID, u.Scheme)
		}
	}
	return m, nil
}

// multiUpstreamProxy will serve requests directed to multiple upstream servers
// registered in the serverMux.
type multiUpstreamProxy struct {
	serveMux *mux.Router
}

// ServerHTTP handles HTTP requests.
func (m *multiUpstreamProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	m.serveMux.ServeHTTP(rw, req)
}

// registerStaticResponseHandler registers a static response handler with at the given path.
func (m *multiUpstreamProxy) registerStaticResponseHandler(upstream options.Upstream) {
	logger.Printf("mapping path %q => static response %d", upstream.Path, derefStaticCode(upstream.StaticCode))
	m.registerSimpleHandler(upstream.Path, newStaticResponseHandler(upstream.ID, upstream.StaticCode))
}

// registerFileServer registers a new fileServer based on the configuration given.
func (m *multiUpstreamProxy) registerFileServer(upstream options.Upstream, u *url.URL) {
	logger.Printf("mapping path %q => file system %q", upstream.Path, u.Path)
	m.registerSimpleHandler(upstream.Path, newFileServer(upstream.ID, upstream.Path, u.Path))
}

// registerHTTPUpstreamProxy registers a new httpUpstreamProxy based on the configuration given.
func (m *multiUpstreamProxy) registerHTTPUpstreamProxy(upstream options.Upstream, u *url.URL, sigData *options.SignatureData, writer pagewriter.Writer) {
	logger.Printf("mapping path %q => upstream %q", upstream.Path, upstream.URI)
	m.registerSimpleHandler(upstream.Path, newHTTPUpstreamProxy(upstream, u, sigData, writer.ProxyErrorHandler))
}

// registerSimpleHandler maintains the behaviour of the go standard serveMux
// by ensuring any path with a trailing `/` matches all paths under that prefix.
func (m *multiUpstreamProxy) registerSimpleHandler(path string, handler http.Handler) {
	if strings.HasSuffix(path, "/") {
		m.serveMux.PathPrefix(path).Handler(handler)
	} else {
		m.serveMux.Path(path).Handler(handler)
	}
}
