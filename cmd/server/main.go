// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/moov-io/auth"
	"github.com/moov-io/base/http/bind"

	"github.com/go-kit/kit/log"
)

var (
	httpAddr  = flag.String("http.addr", bind.HTTP("auth"), "HTTP listen address")
	adminAddr = flag.String("admin.addr", bind.Admin("auth"), "Admin HTTP listen address")

	flagLogFormat = flag.String("log.format", "", "Format for log lines (Options: json, plain")
)

func main() {
	flag.Parse()

	logger := setupLogger(*flagLogFormat)
	logger.Log("startup", fmt.Sprintf("Starting auth server version %s", auth.Version))
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
