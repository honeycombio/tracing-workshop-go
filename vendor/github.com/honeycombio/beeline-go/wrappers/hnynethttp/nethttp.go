package hnynethttp

import (
	"context"
	"net/http"
	"reflect"
	"runtime"

	"github.com/honeycombio/beeline-go/propagation"
	"github.com/honeycombio/beeline-go/timer"
	"github.com/honeycombio/beeline-go/trace"
	"github.com/honeycombio/beeline-go/wrappers/common"
	libhoney "github.com/honeycombio/libhoney-go"
)

// WrapHandler will create a Honeycomb event per invocation of this handler with
// all the standard HTTP fields attached. If passed a ServeMux instead, pull
// what you can from there
func WrapHandler(handler http.Handler) http.Handler {
	// if we can cache handlerName here, let's do so for efficiency's sake
	handlerName := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()

	wrappedHandler := func(w http.ResponseWriter, r *http.Request) {
		// get a new context with our trace from the request, and add common fields
		ctx, span := common.StartSpanOrTraceFromHTTP(r)
		defer span.Send()
		// push the context with our trace and span on to the request
		r = r.WithContext(ctx)
		// replace the writer with our wrapper to catch the status code
		wrappedWriter := common.NewResponseWriter(w)

		mux, ok := handler.(*http.ServeMux)
		if ok {
			// this is actually a mux! let's do extra muxxy stuff
			handler, pat := mux.Handler(r)
			name := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
			hType := reflect.TypeOf(handler).String()
			span.AddField("handler.pattern", pat)
			span.AddField("handler.type", hType)
			if name != "" {
				span.AddField("handler.name", name)
				span.AddField("name", name)
			}
		} else {
			if handlerName != "" {
				span.AddField("handler.name", handlerName)
				span.AddField("name", handlerName)
			}
		}

		handler.ServeHTTP(wrappedWriter.Wrapped, r)
		if wrappedWriter.Status == 0 {
			wrappedWriter.Status = 200
		}
		span.AddField("response.status_code", wrappedWriter.Status)
	}
	return http.HandlerFunc(wrappedHandler)
}

// WrapHandlerFunc will create a Honeycomb event per invocation of this handler
// function with all the standard HTTP fields attached.
func WrapHandlerFunc(hf func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	handlerFuncName := runtime.FuncForPC(reflect.ValueOf(hf).Pointer()).Name()
	return func(w http.ResponseWriter, r *http.Request) {
		// get a new context with our trace from the request, and add common fields
		ctx, span := common.StartSpanOrTraceFromHTTP(r)
		defer span.Send()
		// push the context with our trace and span on to the request
		r = r.WithContext(ctx)
		// replace the writer with our wrapper to catch the status code
		wrappedWriter := common.NewResponseWriter(w)
		// add the name of the handler func we're about to invoke
		if handlerFuncName != "" {
			span.AddField("handler_func_name", handlerFuncName)
			span.AddField("name", handlerFuncName)
		}

		hf(wrappedWriter.Wrapped, r)
		if wrappedWriter.Status == 0 {
			wrappedWriter.Status = 200
		}
		span.AddField("response.status_code", wrappedWriter.Status)
	}
}

type hnyTripper struct {
	// wrt is the wrapped round tripper
	wrt http.RoundTripper
}

func (ht *hnyTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	span := trace.GetSpanFromContext(ctx)
	if span == nil {
		return ht.eventRoundTrip(r)
	}
	return ht.spanRoundTrip(ctx, span, r)
}

func (ht *hnyTripper) eventRoundTrip(r *http.Request) (*http.Response, error) {
	// if there's no trace in the context, just send an event
	tm := timer.Start()
	ev := libhoney.NewEvent()
	defer ev.Send()

	// add in common request headers.
	for k, v := range common.GetRequestProps(r) {
		ev.AddField(k, v)
	}

	ev.AddField("meta.type", "http_client")

	resp, err := ht.wrt.RoundTrip(r)

	if err != nil {
		// TODO should this error field be namespaced somehow
		ev.AddField("error", err.Error())
	}
	dur := tm.Finish()
	ev.AddField("duration_ms", dur)
	return resp, err

}

func (ht *hnyTripper) spanRoundTrip(ctx context.Context, span *trace.Span, r *http.Request) (*http.Response, error) {
	// we have a trace, let's use it and pass along trace context in addition to
	// making a span around this HTTP call
	ctx, span = span.CreateChild(ctx)
	defer span.Send()

	r = r.WithContext(ctx)
	// add in common request headers.
	for k, v := range common.GetRequestProps(r) {
		span.AddField(k, v)
	}
	span.AddField("meta.type", "http_client")
	span.AddField("name", "http_client")
	r.Header.Add(propagation.TracePropagationHTTPHeader, span.SerializeHeaders())

	resp, err := ht.wrt.RoundTrip(r)

	if err != nil {
		// TODO should this error field be namespaced somehow
		span.AddField("error", err.Error())
	} else {
		span.AddField("response.status_code", resp.StatusCode)

	}
	return resp, err
}

// WrapRoundTripper wraps an http transport for outgoing HTTP calls. Using a
// wrapped transport will send an event to Honeycomb for each outbound HTTP call
// you make. Include a context with outbound requests when possible to enable
// correlation
func WrapRoundTripper(r http.RoundTripper) http.RoundTripper {
	return &hnyTripper{
		wrt: r,
	}
}
