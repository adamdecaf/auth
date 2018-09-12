// Copyright 2018 The ACH Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	kitprom "github.com/go-kit/kit/metrics/prometheus"
	stdprom "github.com/prometheus/client_golang/prometheus"
)

var (
	// db is the connection point for making SQL calls to our sqlite database.
	// You use it like any other database/sql driver. Part of shutdown is to close
	// out the file/session.
	//
	// https://github.com/mattn/go-sqlite3/blob/master/_example/simple/simple.go
	// https://astaxie.gitbooks.io/build-web-application-with-golang/en/05.3.html
	db         *sql.DB
	sqlitePath string

	migrations = []string{
		// Initial user setup
		//
		// TODO(adam): be super fancy and generate README.md table in go:generate
		`create table if not exists users(user_id primary key, email, clean_email, created_at timestamp);`,
		`create table if not exists user_approval_codes (user_id primary key, code, valid_until timestamp);`,
		`create table if not exists user_details(user_id primary key, first_name, last_name, phone, company_url);`,

		`create table if not exists user_cookies(user_id primary key, data, valid_until timestamp);`,
		`create table if not exists user_passwords(user_id primary key, password, salt);`,
	}

	// Metrics
	connections = kitprom.NewGaugeFrom(stdprom.GaugeOpts{
		Name: "sqlite_connections",
		Help: "How many sqlite connections and what status they're in.",
	}, []string{"state"})
)

type promMetricCollector struct{}

func (promMetricCollector) run() {
	if db == nil {
		return
	}

	for {
		stats := db.Stats()
		connections.With("state", "idle").Set(float64(stats.Idle))
		connections.With("state", "inuse").Set(float64(stats.InUse))
		connections.With("state", "open").Set(float64(stats.OpenConnections))

		time.Sleep(1)
	}
}

func init() {
	path := os.Getenv("SQLITE_DB_PATH")
	if path == "" || strings.Contains(path, "..") {
		// set default if empty or trying to escape
		// don't filepath.ABS to avoid full-fs reads
		path = "auth.db"
	}

	d, err := sql.Open("sqlite3", path)
	if err != nil {
		err = fmt.Errorf("problem opening sqlite3 file: %v", err)
		logger.Log("sqlite", err)
		panic(err.Error())
	}
	db = d
	sqlitePath = path

	prom := promMetricCollector{}
	go prom.run()
}

func migrate() error {
	logger.Log("sqlite", fmt.Sprintf("migrating %s", sqlitePath))
	for i := range migrations {
		row := migrations[i]
		res, err := db.Exec(row)
		if err != nil {
			return fmt.Errorf("migration #%d [%s...] had problem: %v", i, row[:40], err)
		}
		n, err := res.RowsAffected()
		if err == nil {
			logger.Log("sqlite", fmt.Sprintf("migration #%d [%s...] changed %d rows", i, row[:40], n))
		}
	}
	logger.Log("sqlite", "finished migrations")
	return nil
}