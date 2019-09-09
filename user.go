// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/moov-io/base"
	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

// TODO(adam): validate email (by sending with approval_code) [probably diff file]

// TODO(adam): having a row in user_approval_codes means user isn't verified. Delete on email approval.

type User struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	FirstName  string    `json:"firstName"`
	LastName   string    `json:"lastName"`
	Phone      string    `json:"phone"`
	CompanyURL string    `json:"companyUrl"`
	CreatedAt  base.Time `json:"createdAt"`
}

var (
	dropPlusExtender = regexp.MustCompile(`(\+.*)$`)
	dropPeriods      = strings.NewReplacer(".", "")

	errNoCookieData = errors.New("no cookie data provided")
)

const (
	bcryptCostFactor = 12

	// from 'go doc time Time.String', format used in sqlite columns
	serializedTimestampFormat = "2006-01-02 15:04:05.999999999 -0700 MST"
)

func (u *User) cleanEmail() string {
	return cleanEmail(u.Email)
}

// cleanEmail strips all the funky characters from an email address.
//
// Essentially this boils down to the following pattern (ignoring case):
//   [a-z0-9]@[a-z0-9].[a-z]
//
// Callers should be aware of when an empty string is returned.
func cleanEmail(email string) string {
	// split at '@'
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}

	parts[0] = dropPlusExtender.ReplaceAllString(parts[0], "")
	parts[0] = dropPeriods.Replace(parts[0])

	return strings.ToLower(strings.Join(parts, "@"))
}

// generateID creates a new ID for our auth system.
// Do no assume anything about these ID's other than
// they are strings. Case matters
func generateID() string {
	bs := make([]byte, 20)
	n, err := rand.Read(bs)
	if err != nil || n == 0 {
		logger.Log("generateID", fmt.Sprintf("n=%d, err=%v", n, err))
		return ""
	}
	return strings.ToLower(hex.EncodeToString(bs))
}

func addUserProfileRoutes(r *mux.Router, logger log.Logger, auth authable, userRepo userRepository) {
	r.Methods("PATCH").Path("/users/{user_id}").HandlerFunc(updateUserProfile(logger, auth, userRepo))
}

type userProfileRequest struct {
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Phone      string `json:"phone,omitempty"`
	CompanyURL string `json:"companyUrl,omitempty"`
}

func updateUserProfile(logger log.Logger, auth authable, userRepo userRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "checkLogin")

		userId, err := extractUserId(auth, r)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// read request body
		var req userProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		// grab user records
		user, err := userRepo.lookupByUserId(userId)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}
		if user == nil {
			moovhttp.Problem(w, errors.New("user not found"))
			return
		}

		// Set new values
		if req.FirstName != "" {
			user.FirstName = req.FirstName
		}
		if req.LastName != "" {
			user.LastName = req.LastName
		}
		if req.Phone != "" {
			user.Phone = req.Phone
		}
		if req.CompanyURL != "" {
			user.CompanyURL = req.CompanyURL
		}

		// Write user back
		if err := userRepo.upsert(user); err != nil {
			internalError(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

type userRepository interface {
	// lookupByUserId returns a User according to the id provided.
	// This function can return nil, nil meaning no user was found.
	lookupByUserId(id string) (*User, error)

	// lookupByEmail finds a user by the given email address.
	//
	// This function can return nil, nil meaning no user was found
	lookupByEmail(email string) (*User, error)

	upsert(*User) error
}

type sqliteUserRepository struct {
	db  *sql.DB
	log log.Logger
}

func (repo sqliteUserRepository) close() error {
	return repo.db.Close()
}

func (s *sqliteUserRepository) lookupByUserId(userId string) (*User, error) {
	query := `select u.email, u.created_at, ud.first_name, ud.last_name, ud.phone, ud.company_url
from users as u
inner join user_details as ud
on u.user_id = ud.user_id
where u.user_id = ?
limit 1`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(userId)

	u := &User{}
	u.ID = userId
	var createdAt string // needs parsing
	err = row.Scan(&u.Email, &createdAt, &u.FirstName, &u.LastName, &u.Phone, &u.CompanyURL)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil // no user found
		}
		return nil, err
	}

	t, err := time.Parse(serializedTimestampFormat, createdAt)
	if err != nil {
		s.log.Log("user", fmt.Sprintf("bad users.created_at format %q: %v", createdAt, err))
	}
	u.CreatedAt = base.NewTime(t)
	if u.Email == "" {
		return nil, nil
	}
	return u, nil
}

func (s *sqliteUserRepository) lookupByEmail(email string) (*User, error) {
	query := `select user_id from users where clean_email = ?`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(cleanEmail(email))

	var userId string
	err = row.Scan(&userId)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil // no user found
		}
		return nil, err
	}

	if userId == "" {
		return nil, nil
	}
	return s.lookupByUserId(userId)
}

func (s *sqliteUserRepository) upsert(inc *User) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	// insert/update into 'users'
	query := `replace into users (user_id, email, clean_email, created_at) values (?, ?, ?, ?)`
	stmt, err := tx.Prepare(query)
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("problem preparing users query userId=%s, err=%v, rollback err=%v", inc.ID, err, e)
	}
	_, err = stmt.Exec(inc.ID, inc.Email, inc.cleanEmail(), inc.CreatedAt.Format(serializedTimestampFormat))
	if err != nil {
		stmt.Close()
		e := tx.Rollback()
		return fmt.Errorf("problem upserting users userId=%s, err=%v, rollback err=%v", inc.ID, err, e)
	}
	stmt.Close()

	// insert/update into 'user_details'
	query = `replace into user_details (user_id, first_name, last_name, phone, company_url) values (?, ?, ?, ?, ?)`
	stmt, err = tx.Prepare(query)
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("problem preparing user_details query userId=%s, err=%v, rollback err=%v", inc.ID, err, e)
	}
	_, err = stmt.Exec(inc.ID, inc.FirstName, inc.LastName, inc.Phone, inc.CompanyURL)
	if err != nil {
		stmt.Close()
		e := tx.Rollback()
		return fmt.Errorf("problem upserting user_details userId=%s, err=%v, rollback err=%v", inc.ID, err, e)
	}
	stmt.Close()

	return tx.Commit()
}

// authable represents the interactions of a user's authentication
// status. This boils down to password comparison and cookie data.
type authable interface {
	findUserId(data string) (string, error)
	invalidateCookies(userId string) error
	writeCookie(userId string, cookie *http.Cookie) error

	// checkPassword compares the provided password for the user.
	// a non-nil error is returned if the passwords don't match
	// or that the userId doesn't exist.
	checkPassword(userId string, pass string) error
	writePassword(userId string, pass string) error
}

type auth struct {
	db  *sql.DB
	log log.Logger
}

// findUserId takes cookie data and returns the userId associated
func (a *auth) findUserId(data string) (string, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return "", errNoCookieData
	}

	// the SHA256 checksum is stored, not the actual data.
	data, err := hash(data)
	if err != nil {
		return "", err
	}

	query := `select user_id from user_cookies where data == ? and valid_until > ?`
	stmt, err := a.db.Prepare(query)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	rows, err := stmt.Query(data, time.Now().Format(serializedTimestampFormat))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var userId string
	for rows.Next() {
		if err := rows.Scan(&userId); err != nil {
			return "", err
		}
		if userId != "" {
			return userId, nil
		}
	}
	return "", rows.Err()
}

func (a *auth) invalidateCookies(userId string) error {
	stmt, err := a.db.Prepare(`delete from user_cookies where user_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	res, err := stmt.Exec(userId)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	a.log.Log("user", fmt.Sprintf("deleted %d cookies for userId=%s", n, userId))
	return nil
}

func (a *auth) writeCookie(userId string, cookie *http.Cookie) error {
	query := `insert or replace into user_cookies (user_id, data, valid_until) values (?, ?, ?)`
	stmt, err := a.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// hash the data
	data, err := hash(cookie.Value)
	if err != nil {
		return err
	}
	validUntil := cookie.Expires.Format(serializedTimestampFormat)

	// write row
	stmt.Exec(userId, data, validUntil)
	return nil
}

// fakeBcryptRounds just performs a bcrypt.GenerateFromPassword and then
// a bcrypto.CompareHashAndPassword afterwords. In an attempt to make happy
// and sad paths take "approximately" the same.
func (a *auth) fakeBcryptRounds() {
	id := generateID()
	bcrypt.CompareHashAndPassword([]byte(id), []byte(id))
}

func (a *auth) checkPassword(userId string, incoming string) error {
	stmt, err := a.db.Prepare(`select password, salt from user_passwords where user_id = ?`)
	if err != nil {
		a.fakeBcryptRounds()
		return err
	}
	defer stmt.Close()

	row := stmt.QueryRow(userId)
	var storedPassword, storedSalt string
	if err := row.Scan(&storedPassword, &storedSalt); err != nil {
		a.fakeBcryptRounds()
		return err
	}

	return bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(incoming+storedSalt))
}

// writePassword saves a user's password. This function performs no authn/z and generates a new
// salt (which is also saved).
func (a *auth) writePassword(userId string, pass string) error {
	salt := generateID()
	incomingPassword, err := bcrypt.GenerateFromPassword([]byte(pass+salt), bcryptCostFactor)
	if err != nil {
		return err
	}

	stmt, err := a.db.Prepare(`replace into user_passwords (user_id, password, salt) values (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(userId, string(incomingPassword), salt)
	if err != nil {
		return err
	}
	a.log.Log("user", fmt.Sprintf("userId=%s updated password", userId))
	return nil
}

func hash(in string) (string, error) {
	ss := sha256.New()
	n, err := ss.Write([]byte(in))
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", err
	}
	return hex.EncodeToString(ss.Sum(nil)), nil
}
