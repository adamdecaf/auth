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

// TokenStore wraps oauth2.TokenStore with an underlying *sql.DB provided from NewTokenStoreDB
type TokenStore struct {
	oauth2.TokenStore

	db *sql.DB
}

// NewTokenStoreDB returns a TokenStore wrapped with the underlying sql.DB.
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
//       db, err := oauthdb.NewTokenStoreDB("file:token_store.db")
//   }
//
//
// connString is a Data Source Name (DSN), See the respective driver documentation for parameters.
func NewTokenStoreDB(connString string) (*TokenStore, error) {
	driver := detectDriver(connString)

	db, err := sql.Open(driver, connString)
	if err != nil {
		return nil, fmt.Errorf("problem opening oauth2 token store database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("problem with Ping against *sql.DB %s: %v", driver, err)
	}

	tokenStore := &TokenStore{db: db}
	if err := tokenStore.migrate(); err != nil {
		return nil, err
	}
	return tokenStore, nil
}

// Close shuts down connections to the underlying database
func (ts *TokenStore) Close() error {
	return ts.db.Close()
}

func (ts *TokenStore) migrate() error {
	if ts.db == nil {
		return errors.New("TokenStore: missing underlying *sql.DB")
	}
	queries := []string{
		`create table if not exists oauth2_tokens(client_id, user_id, redirect_uri, scope, code, code_expires_in, access, access_expires_in, refresh, refresh_expires_in, created_at datetime, deleted_at datetime, unique (code, access, refresh) on conflict abort)`,
	}
	return migrate(ts.db, queries)
}

// Create writes an oauth2.TokenInfo into the underlying database.
//
// If an existing token exists (matching code, access, and refresh) then that row will be
// replaced by the incoming token. This is done to update the userId on a given token.
func (ts *TokenStore) Create(info oauth2.TokenInfo) error {
	query := `replace into oauth2_tokens (client_id, user_id, redirect_uri, scope, code, code_expires_in, access, access_expires_in, refresh, refresh_expires_in, created_at)
values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := ts.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("token store: failed to prepare Create: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(info.GetClientID(), info.GetUserID(), info.GetRedirectURI(), info.GetScope(), info.GetCode(), info.GetCodeExpiresIn().String(), info.GetAccess(), info.GetAccessExpiresIn().String(), info.GetRefresh(), info.GetRefreshExpiresIn().String(), time.Now())
	return err
}

// RemoveByCode use the authorization code to delete the token information
// TODO(adam): make sure this is guardded by a userId check
func (ts *TokenStore) RemoveByCode(code string) error {
	query := `update oauth2_tokens set deleted_at = ? where code = ? and deleted_at is null`
	stmt, err := ts.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("token store: failed to prepare RemoveByCode: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), code)
	return err
}

// RemoveByAccess use the access token to delete the token information
func (ts *TokenStore) RemoveByAccess(access string) error {
	query := `update oauth2_tokens set deleted_at = ? where access = ? and deleted_at is null`
	stmt, err := ts.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("token store: failed to prepare RemoveByAccess: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), access)
	return err
}

// RemoveByRefresh use the refresh token to delete the token information
func (ts *TokenStore) RemoveByRefresh(refresh string) error {
	query := `update oauth2_tokens set deleted_at = ? where refresh = ? and deleted_at is null`
	stmt, err := ts.db.Prepare(query)
	if err != nil {
		return fmt.Errorf("token store: failed to prepare RemoveByRefresh: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), refresh)
	return err
}

func queryForRow(db *sql.DB, col, needle string) (oauth2.TokenInfo, error) {
	query := fmt.Sprintf(`select client_id, user_id, redirect_uri, scope, code, code_expires_in, access, access_expires_in, refresh, refresh_expires_in, created_at from oauth2_tokens where %s = ? and deleted_at is null limit 1`, col)
	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("token store: failed to prepare RemoveByRefresh: %v", err)
	}
	defer stmt.Close()

	row := stmt.QueryRow(needle)

	var token models.Token

	var createdAt time.Time
	codeExpiresIn, accessExpiresIn, refreshExpiresIn := "", "", ""

	err = row.Scan(&token.ClientID, &token.UserID, &token.RedirectURI, &token.Scope, &token.Code, &codeExpiresIn, &token.Access, &accessExpiresIn, &token.Refresh, &refreshExpiresIn, &createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("token store: failed on GetByCode: %v", err)
	}

	token.AccessCreateAt = createdAt
	token.CodeCreateAt = createdAt
	token.RefreshCreateAt = createdAt

	dur, err := time.ParseDuration(accessExpiresIn)
	if err == nil {
		token.AccessExpiresIn = dur
	}
	dur, err = time.ParseDuration(codeExpiresIn)
	if err == nil {
		token.CodeExpiresIn = dur
	}
	dur, err = time.ParseDuration(refreshExpiresIn)
	if err == nil {
		token.RefreshExpiresIn = dur
	}

	return &token, nil
}

// GetByCode use the authorization code for token information data
// TODO(adam): make sure this is protected by a userId check
func (ts *TokenStore) GetByCode(code string) (oauth2.TokenInfo, error) {
	return queryForRow(ts.db, "code", code)
}

// GetByAccess use the access token for token information data
func (ts *TokenStore) GetByAccess(access string) (oauth2.TokenInfo, error) {
	return queryForRow(ts.db, "access", access)
}

// GetByRefresh use the refresh token for token information data
func (ts *TokenStore) GetByRefresh(refresh string) (oauth2.TokenInfo, error) {
	return queryForRow(ts.db, "refresh", refresh)
}
