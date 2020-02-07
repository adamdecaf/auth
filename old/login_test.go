// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moov-io/base"
)

func TestLogin__forwardedPreflight(t *testing.T) {
	// setup test databases
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

	// make our request
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/users/login", nil)
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("X-Forwarded-Method", "OPTIONS")

	handler := checkLogin(nil, auth, repo)
	handler(w, r)
	w.Flush()

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if v := w.Header().Get("Access-Control-Allow-Origin"); v != "http://localhost:8080" {
		t.Errorf("got %s", v)
	}
}

func TestLogin__getUserFromCookie(t *testing.T) {
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

	// Write user
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

	// Write user's cookie
	cookie, err := createCookie(userId, auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.writeCookie(userId, cookie); err != nil {
		t.Fatal(err)
	}

	// Setup HTTP request
	req, err := http.NewRequest("GET", "/auth/check", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", fmt.Sprintf("moov_auth=%s", cookie.Value))

	user, err := getUserFromCookie(auth, repo, req)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != user.ID {
		t.Errorf("u.ID=%s user.ID=%s", u.ID, user.ID)
	}
}
