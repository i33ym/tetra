// Package web contains a small web framework extension built over go-chi/chi.
// Handlers return a value (an Encoder) instead of writing to the
// ResponseWriter; a single Respond choke point turns that value — including
// errors — into the HTTP response.
package web

import (
	"context"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Encoder defines behavior that can encode a data model and provide
// the content type for that encoding.
type Encoder interface {
	Encode() (data []byte, contentType string, err error)
}

// HandlerFunc represents a function that handles a http request within our own
// little mini framework.
type HandlerFunc func(ctx context.Context, r *http.Request) Encoder

// Logger represents a function that will be called to add information
// to the logs.
type Logger func(ctx context.Context, msg string, args ...any)

// App is the entrypoint into our application and what configures our context
// object for each of our http handlers.
type App struct {
	log     Logger
	tracer  trace.Tracer
	router  chi.Router
	otmux   http.Handler
	mw      []MidFunc
	origins []string
}

// NewApp creates an App value that handles a set of routes for the application.
func NewApp(log Logger, tracer trace.Tracer, mw ...MidFunc) *App {

	// Create an OpenTelemetry HTTP handler which wraps our router. This starts
	// the initial span and annotates it with information about the request,
	// using the W3C TraceContext standard to set the remote parent when a
	// client request includes the appropriate headers.
	router := chi.NewRouter()

	return &App{
		log:    log,
		tracer: tracer,
		router: router,
		otmux:  otelhttp.NewHandler(router, "request"),
		mw:     mw,
	}
}

// ServeHTTP implements the http.Handler interface. It's the entry point for all
// http traffic and allows the opentelemetry mux to run first to handle tracing.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.origins != nil {
		reqOrigin := r.Header.Get("Origin")
		for _, origin := range a.origins {
			if origin == "*" || origin == reqOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "POST, PATCH, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle pre-flight by sending a 200 OK response.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

	// Clean the request path to avoid redirects on unclean paths that would
	// downgrade POST/PUT/DELETE to GET.
	r.URL.Path = path.Clean(r.URL.Path)

	a.otmux.ServeHTTP(w, r)
}

// EnableCORS enables CORS preflight requests to work.
func (a *App) EnableCORS(origins []string) {
	a.origins = origins
}

// HandlerFuncNoMid sets a handler function for a given HTTP method and path
// pair. It does not include the application middleware or OTEL tracing setup.
func (a *App) HandlerFuncNoMid(method string, group string, path string, handlerFunc HandlerFunc) {
	h := func(w http.ResponseWriter, r *http.Request) {
		ctx := setWriter(r.Context(), w)

		resp := handlerFunc(ctx, r)

		if err := Respond(ctx, w, resp); err != nil {
			a.log(ctx, "web-respond", "ERROR", err)
			return
		}
	}

	a.router.Method(method, mountPath(group, path), http.HandlerFunc(h))
}

// HandlerFunc sets a handler function for a given HTTP method and path pair to
// the application router, wrapping per-route then app-wide middleware.
func (a *App) HandlerFunc(method string, group string, path string, handlerFunc HandlerFunc, mw ...MidFunc) {
	handlerFunc = wrapMiddleware(mw, handlerFunc)
	handlerFunc = wrapMiddleware(a.mw, handlerFunc)

	h := func(w http.ResponseWriter, r *http.Request) {
		ctx := setTracer(r.Context(), a.tracer)
		ctx = setWriter(ctx, w)

		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

		resp := handlerFunc(ctx, r)

		if err := Respond(ctx, w, resp); err != nil {
			a.log(ctx, "web-respond", "ERROR", err)
			return
		}
	}

	a.router.Method(method, mountPath(group, path), http.HandlerFunc(h))
}

// RawHandlerFunc sets a raw handler function for a given HTTP method and path
// pair. The raw handler writes to the ResponseWriter directly (e.g. streaming).
func (a *App) RawHandlerFunc(method string, group string, path string, rawHandlerFunc http.HandlerFunc, mw ...MidFunc) {
	handlerFunc := func(ctx context.Context, r *http.Request) Encoder {
		r = r.WithContext(ctx)
		rawHandlerFunc(GetWriter(ctx), r)
		return nil
	}

	handlerFunc = wrapMiddleware(mw, handlerFunc)
	handlerFunc = wrapMiddleware(a.mw, handlerFunc)

	h := func(w http.ResponseWriter, r *http.Request) {
		ctx := setTracer(r.Context(), a.tracer)
		ctx = setWriter(ctx, w)

		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

		handlerFunc(ctx, r)
	}

	a.router.Method(method, mountPath(group, path), http.HandlerFunc(h))
}

func mountPath(group string, p string) string {
	if group == "" {
		return p
	}
	return "/" + group + p
}
