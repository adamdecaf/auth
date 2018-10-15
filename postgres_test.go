// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func init() {
	// seed pseudorandom generator
	rand.Seed(time.Now().Unix())
}

// port returns a random port between 30,000 and 39,999.
// The returned port is not guarenteed to be free.
func port() int {
	n := rand.Intn(9999)
	return 30000 + n // ports between [30,000 and 39,999)
}

// TestPostgres_port checks to see if we generate ports outside
// the range we expect. (Sanity check)
func TestPostgres_port(t *testing.T) {
	for i := 0; i < 100; i++ {
		n := port()
		if n < 30000 || n >= 40000 {
			t.Fatalf("got %d", n)
		}
	}
}

// createTestPostgres returns a *sql.DB with a random port. Callers
// should be sure to close the db when they're done.
//
// The *os.Process refers to an underlying process running the Postgres
// instance. Callers need to close this when they're done.
//
// TODO(adam): wrap *sql.DB with close that shuts down our docker image
func createTestPostgres(t *testing.T) (*sql.DB, *os.Process, error) {
	t.Helper()

	n := port()

	host, user, dbname, pass := "127.0.0.1", "auth", "auth", "password"
	conf := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, pass, host, n, dbname)

	args := []string{
		"run",
		"-p", fmt.Sprintf("%d:5432", n),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", dbname),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", user),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", pass),
		"-t", "postgres:10.5-alpine",
	}
	cmd := exec.Command("docker", args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	conn, err := createPostgresConnection(conf)
	if err != nil {
		t.Fatal(err)
		return nil, nil, err
	}

	// Wait until db is ready
	attempts := 1
	for {
		attempts++
		if attempts > 10 {
			t.Fatalf("can't connect to postgres after %d attempts", attempts)
		}

		readyLine := "database system is ready to accept connections"
		if strings.Contains(output.String(), readyLine) {
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	return conn, cmd.Process, nil
}

// TODO(adam): helper testXXX(t, db) to test sqlite and postgres.
func TestPostgres__basic(t *testing.T) {
	db, pid, err := createTestPostgres(t)
	if err != nil {
		t.Fatal(err)
		return
	}
	defer db.Close()
	defer pid.Kill()

	// sanity spec
	res, err := db.Query("select 1")
	if err != nil {
		t.Fatalf("%#v\n", err)
	}
	res.Close()
}
