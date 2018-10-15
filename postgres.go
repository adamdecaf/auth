// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	kitprom "github.com/go-kit/kit/metrics/prometheus"
	stdprom "github.com/prometheus/client_golang/prometheus"
)

var (
	// POSTGRES_CONNECTION is a DSN or jdbc string representing a postgres connection.
	// i.e.
	//  - user=pqgotest dbname=pqgotest sslmode=verify-full
	//  - postgres://pqgotest:password@localhost/pqgotest?sslmode=verify-full
	//
	// Docs: https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters
	postgresConnectionString = os.Getenv("POSTGRES_CONNECTION")

	postgresConnections = kitprom.NewGaugeFrom(stdprom.GaugeOpts{
		Name: "postgres_connections",
		Help: "How many postgres connections and what status they're in.",
	}, []string{"state"})
)

func createPostgresConnection(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		err = fmt.Errorf("problem opening postgres connection: %v", err)
		logger.Log("postgres", err)
		return nil, err
	}
	return db, nil
}
