// Copyright 2018 The Moov Authors
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

	"github.com/moov-io/base/admin"
	moovhttp "github.com/moov-io/base/http"
	"github.com/moov-io/base/http/bind"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/gorilla/mux"
	"github.com/mattn/go-sqlite3"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

var (
	httpAddr  = flag.String("http.addr", bind.HTTP("auth"), "HTTP listen address")
	adminAddr = flag.String("admin.addr", bind.Admin("auth"), "Admin HTTP listen address")

	logger        log.Logger
	flagLogFormat = flag.String("log.format", "", "Format for log lines (Options: json, plain")

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
	routeHistogram = prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
		Name: "http_response_duration_seconds",
		Help: "Histogram representing the http response durations",
	}, []string{"route"})

	clientGenerations = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "oauth2_client_generations",
		Help: "Count of auth tokens created",
	}, nil)
	tokenGenerations = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "oauth2_token_generations",
		Help: "Count of auth tokens created",
	}, nil)
)

func main() {
	flag.Parse()

	// Setup logging, default to stdout
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		*flagLogFormat = v
	}
	if strings.ToLower(*flagLogFormat) == "json" {
		logger = log.NewJSONLogger(os.Stderr)
	} else {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)
	logger.Log("startup", fmt.Sprintf("Starting auth server version %s", Version))

	// Listen for application termination.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		logger.Log("exit", fmt.Sprintf("caught signal: %v", <-c))
		os.Exit(0)
	}()

	// Check to see if our -admin.addr flag has been overridden
	if v := os.Getenv("HTTP_ADMIN_BIND_ADDRESS"); v != "" {
		*adminAddr = v
	}

	adminServer := admin.NewServer(*adminAddr)
	go func() {
		logger.Log("admin", fmt.Sprintf("listening on %s", adminServer.BindAddr()))
		if err := adminServer.Listen(); err != nil {
			logger.Log("admin", fmt.Errorf("problem starting admin http: %v", err))
			os.Exit(1)
		}
	}()
	defer adminServer.Shutdown()

	// migrate database
	if sqliteVersion, _, _ := sqlite3.Version(); sqliteVersion != "" {
		logger.Log("main", fmt.Sprintf("sqlite version %s", sqliteVersion))
	}
	db, err := createConnection(getSqlitePath())
	if err != nil {
		logger.Log("main", fmt.Errorf("database connection error: %v", err))
		os.Exit(1)
	}
	if err := migrate(db, logger); err != nil {
		logger.Log("main", fmt.Errorf("database migration error: %v", err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Log("main", fmt.Errorf("database Close error: %v", err))
			os.Exit(1)
		}
	}()

	clientStore, err := setupOAuthClientStore(os.Getenv("OAUTH2_CLIENTS_DSN"))
	if err != nil {
		logger.Log("main", fmt.Sprintf("Failed to setup OAuth2 client store: %v", err))
		os.Exit(1)
	}
	tokenStore, err := setupOAuthTokenStore(os.Getenv("OAUTH2_TOKENS_DSN"))
	if err != nil {
		logger.Log("main", fmt.Sprintf("Failed to setup OAuth2 token store: %v", err))
		os.Exit(1)
	}
	oauth, err := setupOAuthServer(logger, clientStore, tokenStore)
	if err != nil {
		logger.Log("main", fmt.Sprintf("Failed to setup OAuth2 service: %v", err))
		os.Exit(1)
	}
	defer func() {
		if err := oauth.shutdown(); err != nil {
			logger.Log("main", fmt.Errorf("oauth shutdown error: %v", err))
			os.Exit(1)
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
	moovhttp.AddCORSHandler(router)
	addPingRoute(router)
	addAuthRoutes(router, logger, authService, oauth, userService)
	addOAuthRoutes(router, oauth, logger, authService)
	addLoginRoutes(router, logger, authService, userService)
	addLogoutRoutes(router, logger, authService)
	addSignupRoutes(router, logger, authService, userService)
	addUserProfileRoutes(router, logger, authService, userService)

	// Check to see if our -http.addr flag has been overridden
	if v := os.Getenv("HTTP_BIND_ADDRESS"); v != "" {
		*httpAddr = v
	}

	serve := &http.Server{
		Addr:    *httpAddr,
		Handler: router,
		TLSConfig: &tls.Config{
			InsecureSkipVerify:       false,
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS12,
		},
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
	}
	shutdownServer := func() {
		if err := serve.Shutdown(context.TODO()); err != nil {
			logger.Log("main", fmt.Errorf("server shutdown: %v", err))
			os.Exit(1)
		}
	}
	defer shutdownServer()

	// Block on HTTP server bind
	if serveViaTLS {
		logger.Log("transport", "HTTPS", "addr", *httpAddr)
		if err := serve.ListenAndServeTLS(tlsCertificate, tlsPrivateKey); err != nil {
			logger.Log("main", fmt.Errorf("ListenAndServeTLS: %v", err))
			os.Exit(1)
		}
	} else {
		logger.Log("transport", "HTTP", "addr", *httpAddr)
		if err := serve.ListenAndServe(); err != nil {
			logger.Log("main", fmt.Errorf("ListenAndServe: %v", err))
			os.Exit(1)
		}
	}

	os.Exit(1)
}
