// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moov-io/auth"
	"github.com/moov-io/auth/internal/kratos"
	"github.com/moov-io/auth/internal/util"
	"github.com/moov-io/base/admin"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/base/http/bind"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/ory/kratos-client-go/client"
)

var (
	flagHttpAddr  = flag.String("http.addr", bind.HTTP("auth"), "HTTP listen address")
	flagAdminAddr = flag.String("admin.addr", bind.Admin("auth"), "Admin HTTP listen address")

	flagLogFormat = flag.String("log.format", "", "Format for log lines (Options: json, plain")

	flagCertFile = flag.String("http.tls.cert", "", "Filepath for TLS certificate")
	flagKeyFile  = flag.String("http.tls.key", "", "Filepath for TLS private key")
)

func main() {
	flag.Parse()

	logger := setupLogger(*flagLogFormat)
	logger.Log("main", fmt.Sprintf("Starting auth server version %s", auth.Version))

	// Listen for application termination.
	errs := make(chan error)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	adminServer := setupAdminServer(logger, util.Or(os.Getenv("HTTP_ADMIN_BIND_ADDRESS"), *flagAdminAddr))
	defer adminServer.Shutdown()

	kratosClient := setupKratosClient(adminServer)
	fmt.Println(kratosClient)

	handler, httpServer := setupHTTPServer(logger, util.Or(os.Getenv("HTTP_BIND_ADDRESS"), *flagHttpAddr))
	defer func(svc *http.Server) {
		if err := svc.Shutdown(context.TODO()); err != nil {
			logger.Log("main", err)
		}
	}(httpServer)

	addPingRoute(logger, handler)

	go startHTTPServer(logger, httpServer, errs)

	if err := <-errs; err != nil {
		logger.Log("main", fmt.Sprintf("shutdown: %v", err))
		os.Exit(1)
	}
}

func setupLogger(format string) (logger log.Logger) {
	if strings.EqualFold(format, "json") {
		logger = log.NewJSONLogger(os.Stderr)
	} else {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)
	return logger
}

func setupAdminServer(logger log.Logger, addr string) *admin.Server {
	adminServer := admin.NewServer(addr)
	adminServer.AddVersionHandler(auth.Version) // Setup 'GET /version'

	go func() {
		logger.Log("admin", fmt.Sprintf("listening on %s", adminServer.BindAddr()))
		if err := adminServer.Listen(); err != nil {
			logger.Log("admin", fmt.Sprintf("problem starting admin http: %v", err))
		}
	}()

	return adminServer
}

func setupKratosClient(adminServer *admin.Server) *client.OryKratos {
	c := kratos.New()
	adminServer.AddLivenessCheck("kratos", func() error {
		_, err := c.Health.IsInstanceReady(nil)
		return err
	})
	return c
}

func setupHTTPServer(logger log.Logger, addr string) (*mux.Router, *http.Server) {
	r := mux.NewRouter()
	return r, &http.Server{
		Addr:    addr,
		Handler: r,
		TLSConfig: &tls.Config{
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS12,
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func startHTTPServer(logger log.Logger, httpServer *http.Server, errs chan error) {
	certFile := util.Or(os.Getenv("HTTPS_CERT_FILE"), *flagCertFile)
	keyFile := util.Or(os.Getenv("HTTPS_KEY_FILE"), *flagKeyFile)

	if certFile != "" && keyFile != "" {
		logger.Log("startup", fmt.Sprintf("binding to %s for secure HTTP server", httpServer.Addr))
		if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil {
			errs <- err
		}
	} else {
		logger.Log("startup", fmt.Sprintf("binding to %s for HTTP server", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil {
			errs <- err
		}
	}
}

func addPingRoute(logger log.Logger, r *mux.Router) {
	r.Methods("GET").Path("/ping").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestID := moovhttp.GetRequestID(r); requestID != "" {
			logger.Log("route", "ping", "requestID", requestID)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PONG"))
	})
}
