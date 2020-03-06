// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moov-io/base"

	"github.com/go-kit/kit/log"
)

type testAuth struct {
	auth

	dir string
}

func (a *testAuth) cleanup() error {
	if err := a.auth.db.Close(); err != nil {
		return err
	}
	return os.RemoveAll(a.dir)
}

// createTestAuthable returns a new auth instance wrapped with a cleanup() method.
// The sqlite database is created in a temp directory.
func createTestAuthable() (*testAuth, error) {
	dir, err := ioutil.TempDir("", "auth")
	if err != nil {
		return nil, err
	}

	db, err := createConnection(filepath.Join(dir, "auth.db"))
	if err != nil {
		return nil, err
	}

	logger := log.NewLogfmtLogger(ioutil.Discard)
	if err := migrate(db, logger); err != nil {
		return nil, err
	}

	return &testAuth{auth{db, logger}, dir}, nil
}

type testUserRepository struct {
	sqliteUserRepository

	dir string
}

func (repo *testUserRepository) cleanup() error {
	if err := repo.sqliteUserRepository.close(); err != nil {
		return err
	}
	return os.RemoveAll(repo.dir)
}

// createTestUserRepository returns a sqliteUserRepository wrapped with cleanup
// for our test database.
func createTestUserRepository() (*testUserRepository, error) {
	dir, err := ioutil.TempDir("", "userRepository")
	if err != nil {
		return nil, err
	}

	db, err := createConnection(filepath.Join(dir, "auth.db"))
	if err != nil {
		return nil, err
	}

	logger := log.NewLogfmtLogger(ioutil.Discard)
	if err := migrate(db, logger); err != nil {
		return nil, err
	}

	return &testUserRepository{sqliteUserRepository{db, logger}, dir}, nil
}

func TestUser__cleanEmail(t *testing.T) {
	cases := []struct {
		input, expected string
	}{
		{"john.doe+moov@gmail.com", "johndoe@gmail.com"},
		{"john.doe+@gmail.com", "johndoe@gmail.com"},
		{"john.doe@gmail.com", "johndoe@gmail.com"},
		{"john.doe@gmail.com", "johndoe@gmail.com"},
		{"john+moov@gmail.com", "john@gmail.com"},
		{"john.@gmail.com", "john@gmail.com"},
		{"john.+@gmail.com", "john@gmail.com"},
	}

	for i := range cases {
		res := cleanEmail(cases[i].input)
		if res != cases[i].expected {
			t.Errorf("got %q", res)
		}
	}
}

func TestUserRepository(t *testing.T) {
	repo, err := createTestUserRepository()
	if err != nil {
		t.Fatal(err)
	}
	defer repo.cleanup()

	u, err := repo.lookupByUserId(generateID())
	if err != nil {
		t.Error(err)
	}
	if u != nil {
		t.Errorf("expected no user, got %v", u)
	}

	u, err = repo.lookupByEmail(generateID())
	if err != nil {
		t.Error(err)
	}
	if u != nil {
		t.Errorf("expected no user, got %v", u)
	}

	// create and insert user
	userId := generateID()
	u = &User{
		ID:        userId,
		Email:     "test@moov.io",
		FirstName: "Jane",
		LastName:  "Doe",
		Phone:     "111.222.3333",
		CreatedAt: base.NewTime(time.Now().Add(-1 * time.Second)),
	}

	if err := repo.upsert(u); err != nil {
		t.Fatal(err)
	}

	// make sure user was written
	uu, err := repo.lookupByUserId(userId)
	if err != nil {
		t.Error(err)
	}
	if uu == nil {
		t.Error("expected user")
	}

	uu, err = repo.lookupByEmail(u.Email)
	if err != nil {
		t.Error(err)
	}
	if uu == nil {
		t.Error("expected user")
	}

	// test upsert
	u.FirstName = "John"
	u.LastName = "Doe"
	u.Phone = "222.111.3333"
	u.CompanyURL = "https://moov.io"
	if err := repo.upsert(u); err != nil {
		t.Fatal(err)
	}

	// verify
	uu, err = repo.lookupByUserId(userId)
	if err != nil {
		t.Error(err)
	}
	if uu == nil {
		t.Fatal("expected user")
	}
	if u.ID != uu.ID {
		t.Errorf("u.ID=%q, uu.ID=%q", u.ID, uu.ID)
	}
	if u.FirstName != uu.FirstName {
		t.Errorf("u.FirstName=%q, uu.FirstName=%q", u.FirstName, uu.FirstName)
	}
	if u.LastName != uu.LastName {
		t.Errorf("u.LastName=%q, uu.LastName=%q", u.LastName, uu.LastName)
	}
	if u.Phone != uu.Phone {
		t.Errorf("u.Phone=%q, uu.Phone=%q", u.Phone, uu.Phone)
	}
	if u.CompanyURL != uu.CompanyURL {
		t.Errorf("u.CompanyURL=%q, uu.CompanyURL=%q", u.CompanyURL, uu.CompanyURL)
	}
	if !u.CreatedAt.Equal(uu.CreatedAt) {
		t.Errorf("u.CreatedAt=%q, uu.CreatedAt=%q", u.CreatedAt, uu.CreatedAt)
	}
}

func TestUser__update(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	repo, err := createTestUserRepository()
	if err != nil {
		t.Fatal(err)
	}
	defer repo.cleanup()

	// create and insert user
	userId := generateID()
	u := &User{
		ID:        userId,
		Email:     "test@moov.io",
		FirstName: "Jane",
		LastName:  "Doe",
		Phone:     "111.222.3333",
		CreatedAt: base.NewTime(time.Now().Add(-1 * time.Second)),
	}
	if err := repo.upsert(u); err != nil {
		t.Fatal(err)
	}

	// Create test cookie
	cookie, err := createCookie(userId, auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.writeCookie(userId, cookie); err != nil {
		t.Fatal(err)
	}

	// Make our PATCH request
	var body bytes.Buffer
	err = json.NewEncoder(&body).Encode(userProfileRequest{
		FirstName:  "first",
		LastName:   "last",
		Phone:      "123.456.7890",
		CompanyURL: "https://moov.io",
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", fmt.Sprintf("/users/%s", userId), &body)
	r.Header.Set("X-User-Id", userId)
	r.Header.Set("X-Request-Id", generateID())
	r.Header.Set("Cookie", fmt.Sprintf("moov_auth=%s", cookie.Value))

	updateUserProfile(log.NewNopLogger(), auth, repo)(w, r)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("got %d: %v", w.Code, w.Body.String())
	}
}
