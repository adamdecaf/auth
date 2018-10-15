// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	kitprom "github.com/go-kit/kit/metrics/prometheus"
)

var (
	// migrations holds all our SQL migrations to be done (in order)
	migrations = []string{
		// Initial user setup
		//
		// TODO(adam): be super fancy and generate README.md table in go:generate
		`create table if not exists users(user_id primary key, email, clean_email, created_at);`,
		`create table if not exists user_approval_codes (user_id primary key, code, valid_until);`,
		`create table if not exists user_details(user_id primary key, first_name, last_name, phone, company_url);`,
		`create table if not exists user_cookies(user_id primary key, data, valid_until);`,
		`create table if not exists user_passwords(user_id primary key, password, salt);`,
	}
)

type promMetricCollector struct{}

func (p *promMetricCollector) run(db *sql.DB, m *kitprom.Gauge) {
	if db == nil {
		return
	}
	tick := time.NewTicker(1 * time.Second)

	for range tick.C {
		stats := db.Stats()

		// sqlite connection stats
		m.With("state", "idle").Set(float64(stats.Idle))
		m.With("state", "inuse").Set(float64(stats.InUse))
		m.With("state", "open").Set(float64(stats.OpenConnections))
	}
}

// migrate runs our database migrations (defined at the top of this file)
// over a sqlite database it creates first.
// To configure where on disk the sqlite db is set SQLITE_DB_PATH.
//
// You use db like any other database/sql driver.
//
// https://github.com/mattn/go-sqlite3/blob/master/_example/simple/simple.go
// https://astaxie.gitbooks.io/build-web-application-with-golang/en/05.3.html
func migrate(db *sql.DB, logger log.Logger) error {
	logger.Log("sqlite", "starting sqlite migrations...") // TODO(adam): more detail?
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
