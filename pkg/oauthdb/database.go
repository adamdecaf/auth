// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package oauthdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func detectDriver(dsn string) string {
	idx := strings.Index(dsn, ":")
	if idx < 1 {
		return "unknown"
	}

	switch dsn[:idx] {
	case "file":
		// TODO(adam): probably wrong for other file-based databases
		return "sqlite3"
	default:
		return "unknown"
	}
}

// migrate runs "database migrations" over the provided *sql.DB
func migrate(db *sql.DB, queries []string) error {
	if len(queries) == 0 {
		return errors.New("no database migrations")
	}

	for i, query := range queries {
		stmt, err := db.Prepare(query)
		if err != nil {
			return fmt.Errorf("migration #%d [%s...] didn't prepare: %v", i, query[:40], err)
		}

		res, err := stmt.Exec()
		if err != nil {
			return fmt.Errorf("migration #%d [%s...] had problem: %v", i, query[:40], err)
		}

		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("migration #%d [%s...] changed %d rows", i, query[:40], n)
		}

		stmt.Close()
	}
	return nil
}

// generateID creates a new random ID
func generateID() string {
	bs := make([]byte, 20)
	n, err := rand.Read(bs)
	if err != nil || n == 0 {
		return ""
	}
	return strings.ToLower(hex.EncodeToString(bs))
}
