// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package kratos

import (
	"os"
	"strings"

	"github.com/moov-io/auth/internal/util"

	"github.com/go-openapi/strfmt"
	// "github.com/ory/kratos-client-go/go/client"
	"github.com/ory/kratos-client-go/client"
)

func New() *client.OryKratos {
	return setupKratosClient(
		os.Getenv("KRATOS_HOST"),
		os.Getenv("KRATOS_BASE_PATH"),
		os.Getenv("KRATOS_SCHEMES"),
	)
}

func setupKratosClient(host, basePath, schemes string) *client.OryKratos {
	cfg := &client.TransportConfig{
		Host:     util.Or(host, "localhost:4433"),
		BasePath: util.Or(basePath, "/"),
		Schemes:  []string{"http"}, // client.DefaultSchemes,
	}
	if schemes != "" {
		cfg.Schemes = strings.Split(schemes, ",")
	}
	return client.NewHTTPClientWithConfig(strfmt.NewFormats(), cfg)
}
