// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moov-io/auth"
	"github.com/moov-io/auth/internal/kratos"
	"github.com/moov-io/auth/internal/util"
	"github.com/moov-io/base/admin"
	"github.com/moov-io/base/http/bind"

	"github.com/go-kit/kit/log"
	"github.com/ory/kratos-client-go/client"
)

var (
	flagHttpAddr  = flag.String("http.addr", bind.HTTP("auth"), "HTTP listen address")
	flagAdminAddr = flag.String("admin.addr", bind.Admin("auth"), "Admin HTTP listen address")

	flagLogFormat = flag.String("log.format", "", "Format for log lines (Options: json, plain")
)

func main() {
	flag.Parse()

	logger := setupLogger(*flagLogFormat)
	logger.Log("startup", fmt.Sprintf("Starting auth server version %s", auth.Version))

	adminServer := setupAdminServer(logger, util.Or(os.Getenv("HTTP_ADMIN_BIND_ADDRESS"), *flagAdminAddr))
	defer adminServer.Shutdown()

	kratosClient := setupKratosClient(adminServer)
	fmt.Println(kratosClient)

	time.Sleep(60 * time.Second)
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
