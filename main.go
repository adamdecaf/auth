// Copyright 2018 The ACH Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moov-io/auth/admin"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/gorilla/mux"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const Version = "v0.1.0-dev"

var (
	httpAddr = flag.String("http.addr", ":8080", "HTTP listen address")

	logger log.Logger

	// Configuration
	tlsCertificate, tlsPrivateKey = os.Getenv("TLS_CERT"), os.Getenv("TLS_KEY")
	serveViaTLS                   = tlsCertificate != "" && tlsPrivateKey != ""

	// Metrics
	// TODO(adam): be super fancy and generate README.md table in go:generate
	authSuccesses = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "auth_successes",
		Help: "Count of successful authorizations",
	}, []string{"method"})
	authFailures = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "auth_failures",
		Help: "Count of failed authorizations",
	}, []string{"method"})
	authInactivations = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "auth_inactivations",
		Help: "Count of inactivated auths (i.e. user logout)",
	}, []string{"method"})

	internalServerErrors = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "http_errors",
		Help: "Count of how many 5xx errors we send out",
	}, nil)

	tokenGenerations = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "auth_token_generations",
		Help: "Count of auth tokens created",
	}, []string{"method"})
)

func main() {
	flag.Parse()

	// Setup logging, default to stdout
	logger = log.NewLogfmtLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)
	logger.Log("startup", fmt.Sprintf("Starting auth server version %s", Version))

	// Listen for application termination.
	errs := make(chan error)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	go admin.Init()

	// migrate database
	db, err := migrate(logger)
	if err != nil {
		logger.Log("sqlite", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Log("sqlite", err)
		}
	}()

	oauth, err := setupOauthServer(logger)
	if err != nil {
		logger.Log("oauth", err)
		errs <- err
	}
	defer func() {
		if err := oauth.shutdown(); err != nil {
			logger.Log("oauth", err)
		}
	}()

	// user services
	authService := &auth{
		db:  db,
		log: logger,
	}
	userService := &sqliteUserRepository{
		db:  db,
		log: logger,
	}

	// api routes
	router := mux.NewRouter()
	addOAuthRoutes(router, oauth, logger, authService)
	addLoginRoutes(router, logger, authService, userService)
	addLogoutRoutes(router, logger, authService)
	addSignupRoutes(router, logger, authService, userService)

	readTimeout, _ := time.ParseDuration("30s")
	writTimeout, _ := time.ParseDuration("30s")
	idleTimeout, _ := time.ParseDuration("60s")

	serve := &http.Server{
		Addr:    *httpAddr,
		Handler: router,
		TLSConfig: &tls.Config{
			InsecureSkipVerify:       false,
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS12,
		},
		ReadTimeout:  readTimeout,
		WriteTimeout: writTimeout,
		IdleTimeout:  idleTimeout,
	}
	shutdownServer := func() {
		if err := serve.Shutdown(nil); err != nil {
			logger.Log("shutdown", err)
		}
	}
	defer shutdownServer()

	adminService := admin.SetupServer()
	defer adminService.Shutdown()

	go func() {
		logger.Log("admin", fmt.Sprintf("Starting admin service on %s", adminService.BindAddress()))
		if err := adminService.Listen(); err != nil {
			logger.Log("admin", "shutting down", "error", err)
		}
	}()

	go func() {
		if serveViaTLS {
			logger.Log("transport", "HTTPS", "addr", *httpAddr)
			if err := serve.ListenAndServeTLS(tlsCertificate, tlsPrivateKey); err != nil {
				logger.Log("main", err)
			}
		} else {
			logger.Log("transport", "HTTP", "addr", *httpAddr)
			if err := serve.ListenAndServe(); err != nil {
				logger.Log("main", err)
			}
		}
	}()

	if err := <-errs; err != nil {
		logger.Log("exit", err)
	}
	os.Exit(0)
}
