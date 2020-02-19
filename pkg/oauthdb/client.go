// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package oauthdb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/oauth2.v3"
	"gopkg.in/oauth2.v3/models"
)

// NewClientStoreDB returns a ClientStore wrapped with the underlying sql.DB.
//
// In order to call this method register your driver.
//
//   import (
//       "database/sql"
//       _ "github.com/mattn/go-sqlite3"
//      "github.com/moov-io/pkg/oauthdb"
//   )
//
//   func main() {
//       db, err := oauthdb.NewClientStoreDB("file:client_store.db")
//   }
//
// connString is a Data Source Name (DSN), See the respective driver documentation for parameters.
func NewClientStoreDB(connString string) (*ClientStore, error) {
	driver := detectDriver(connString)

	db, err := sql.Open(driver, connString)
	if err != nil {
		return nil, fmt.Errorf("problem opening oauth2 client store database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("problem with Ping against *sql.DB %s: %v", driver, err)
	}

	clientStore := &ClientStore{
		db: db,
	}

	if err := clientStore.migrate(); err != nil {
		return nil, err
	}

	return clientStore, nil
}

// ClientStore wraps oauth2.ClientStore with an underlying *sql.DB provided from NewClientStoreDB
type ClientStore struct {
	oauth2.ClientStore

	db *sql.DB
}

func (cs *ClientStore) migrate() error {
	if cs.db == nil {
		return errors.New("ClientStore: missing underlying *sql.DB")
	}
	queries := []string{
		`create table if not exists oauth2_clients(id, secret, domain, user_id, created_at datetime, deleted_at datetime)`,
		`delete from oauth2_clients where deleted_at is not null;`,
	}
	return migrate(cs.db, queries)
}

// Close shuts down connections to the underlying database
func (cs *ClientStore) Close() error {
	return cs.db.Close()
}

// GetByID returns an oauth2.ClientInfo if the ID matches id.
func (cs *ClientStore) GetByID(id string) (oauth2.ClientInfo, error) {
	query := `select id, secret, domain, user_id from oauth2_clients where id = ? and deleted_at is null order by created_at desc limit 1;`
	stmt, err := cs.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("client store: failed to prepare GetByID: %v", err)
	}
	defer stmt.Close()

	row := stmt.QueryRow(id)

	var client models.Client
	if err := row.Scan(&client.ID, &client.Secret, &client.Domain, &client.UserID); err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("readClient: row.Scan: %v", err)
	}

	return &client, nil
}

// Set writes the oauth2.ClientInfo to the underlying database.
func (cs *ClientStore) Set(id string, cli oauth2.ClientInfo) error {
	if cli == nil {
		return fmt.Errorf("nil oauth2.ClientInfo: %T", cli)
	}

	query := `insert into oauth2_clients (id, secret, domain, user_id, created_at) values (?, ?, ?, ?, ?);`
	stmt, err := cs.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("client store: failed to prepare Set: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(cli.GetID(), cli.GetSecret(), cli.GetDomain(), cli.GetUserID(), time.Now())
	return err
}

// GetByUserID returns an array of oauth2.ClientInfo which have a UserID mathcing
// userId.
// If return values are nil that means no matching records were found.
func (cs *ClientStore) GetByUserID(userId string) ([]oauth2.ClientInfo, error) {
	query := `select id, secret, domain, user_id from oauth2_clients where user_id = ? and deleted_at is null order by created_at desc limit 1;`
	stmt, err := cs.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("client store: failed to prepare GetByID: %v", err)
	}
	defer stmt.Close()

	rows, err := stmt.Query(userId)
	if err != nil {
		return nil, fmt.Errorf("client store: failed to query GetByID: %v", err)
	}
	defer rows.Close()

	var clients []oauth2.ClientInfo
	for rows.Next() {
		var client models.Client
		if err := rows.Scan(&client.ID, &client.Secret, &client.Domain, &client.UserID); err != nil {
			if strings.Contains(err.Error(), "no rows in result set") {
				continue // not found
			}
			return nil, fmt.Errorf("readClient: rows.Scan: %v", err)
		}
		clients = append(clients, &client)
	}
	return clients, rows.Err()
}

// DeleteByID removes the oauth2.ClientInfo for the provided id.
func (cs *ClientStore) DeleteByID(id string) error {
	query := `delete from oauth2_clients where id = ?;`
	stmt, err := cs.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("client store: failed to prepare DeleteByID: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	return err
}
