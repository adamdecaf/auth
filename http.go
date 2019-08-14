// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	moovhttp "github.com/moov-io/base/http"

	"github.com/gorilla/mux"
)

const (
	// maxReadBytes is the number of bytes to read
	// from a request body. It's intended to be used
	// with an io.LimitReader
	maxReadBytes = 1 * 1024 * 1024

	cookieName = "moov_auth"
	cookieTTL  = 30 * 24 * time.Hour // days * hours/day * hours
)

var (
	// Domain is the domain to publish cookies under.
	// If empty "localhost" is used.
	//
	// The path is always set to /.
	Domain string = os.Getenv("DOMAIN")
)

func init() {
	if Domain == "" {
		Domain = "localhost"
	}
}

// read consumes an io.Reader (wrapping with io.LimitReader)
// and returns either the resulting bytes or a non-nil error.
func read(r io.Reader) ([]byte, error) {
	r = io.LimitReader(r, maxReadBytes)
	return ioutil.ReadAll(r)
}

func internalError(w http.ResponseWriter, err error) {
	internalServerErrors.Add(1)

	file := moovhttp.InternalError(w, err)
	component := strings.Split(file, ".go")[0]

	if logger != nil {
		logger.Log(component, err, "source", file)
	}
}

// extractCookie attempts to pull out our cookie from the incoming request.
// We use the contents to find the associated userId.
func extractCookie(r *http.Request) *http.Cookie {
	if r == nil {
		return nil
	}
	cs := r.Cookies()
	for i := range cs {
		if cs[i].Name == cookieName {
			return cs[i]
		}
	}
	return nil
}

// createCookie generates a new cookie and associates it with the provided
// userId.
func createCookie(userId string, auth authable) (*http.Cookie, error) {
	cookie := &http.Cookie{
		Domain:   Domain,
		Expires:  time.Now().Add(cookieTTL),
		HttpOnly: false,
		Name:     cookieName,
		Path:     "/",
		Secure:   serveViaTLS,
		Value:    generateID(),
	}
	if err := auth.writeCookie(userId, cookie); err != nil {
		return nil, err
	}
	return cookie, nil
}

func addPingRoute(r *mux.Router) {
	r.Methods("GET").Path("/ping").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "ping")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PONG"))
	})
}

// responsewriter is an http.ResponseWriter that records the
// outgoing response and executes a callback with the saved
// metadata.
type responseWriter struct {
	rec *httptest.ResponseRecorder
	w   http.ResponseWriter

	start          time.Time
	method         string
	request        *http.Request
	headersWritten bool
}

func (w *responseWriter) requestID() string {
	return moovhttp.GetRequestID(w.request)
}

// Header returns headers added to the response
func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}

// Write proxies the provided data to the underlying http.ResponseWriter
func (w *responseWriter) Write(b []byte) (int, error) {
	return w.w.Write(b)
}

// WriteHeader sets the status code for the request and flushes headers
// back to the client.
//
// The provided callback is executed after flushing headers.
func (w *responseWriter) WriteHeader(code int) {
	if w.headersWritten {
		return
	}
	w.headersWritten = true

	moovhttp.SetAccessControlAllowHeaders(w, w.request.Header.Get("Origin"))

	w.rec.WriteHeader(code)
	w.w.WriteHeader(code)

	w.callback()
}

func (w *responseWriter) callback() {
	diff := time.Since(w.start)
	routeHistogram.With("route", w.method).Observe(diff.Seconds())

	if w.method != "" && w.requestID() != "" {
		line := strings.Join([]string{
			fmt.Sprintf("method=%s", w.request.Method),
			fmt.Sprintf("path=%s", w.request.URL.Path),
			fmt.Sprintf("status=%d", w.rec.Code),
			fmt.Sprintf("took=%s", diff),
			fmt.Sprintf("requestID=%s", w.requestID()),
		}, ", ")
		if logger != nil {
			logger.Log(w.method, line)
		}
	}
}

// wrapResponseWriter creates a new responseWriter and initializes an
// httptest.ResponseRecorder
func wrapResponseWriter(w http.ResponseWriter, r *http.Request, method string) http.ResponseWriter {
	return &responseWriter{
		method:  method,
		request: r,
		rec:     httptest.NewRecorder(),
		w:       w,
		start:   time.Now(),
	}
}

// allowForOptions checks if the request was forwarded as part of an OPTIONS pre-flight check.
// This happens because our LB setup uses "forward auth" which means traefik issues an internal
// HTTP request to this method that checks each request for a valid cookie.
//
// Thus, an OPTIONS request hitting traefik creates an internal forward auth call
//  i.e. GET /auth/check (at time of writing)
// which his the auth app. We need to respond '200 OK' in such a case. Pre-flight requests don't
// contain a Cookie (otherwise a client would leak Cookies)
//
// The ResponseWriter will be completed with '200 OK' to pass pre-flight checks and CORS headers
// will be written.
func allowForOptions(w http.ResponseWriter, r *http.Request) bool {
	// Traefik sets several X-Forwarded-* headers for us (and strips them from the public internet)
	origMethod := r.Header.Get("X-Forwarded-Method")
	if strings.EqualFold(origMethod, "OPTIONS") {
		moovhttp.SetAccessControlAllowHeaders(w, r.Header.Get("Origin"))
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}
