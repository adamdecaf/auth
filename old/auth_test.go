// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestAuth__extractUserId(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	knownUserId := generateID()

	// Write our user
	cookie, err := createCookie(knownUserId, auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.writeCookie(knownUserId, cookie); err != nil {
		t.Fatal(err)
	}

	// happy path
	req := httptest.NewRequest("GET", "/users/login", nil)
	req.Header.Set("cookie", fmt.Sprintf("moov_auth=%s", cookie.Value))

	// make our request
	userId, err := extractUserId(auth, req)
	if err != nil {
		t.Error(err)
	}
	if userId == "" {
		t.Error("empty userId")
	}
	if userId != knownUserId {
		t.Errorf("got %q, expected %q", userId, knownUserId)
	}

	// sad path
	req.Header.Set("cookie", "moov_auth=bad")
	userId, err = extractUserId(auth, req)
	if err == nil {
		t.Error("expected error")
	}
	if userId != "" {
		t.Errorf("got %s", userId)
	}
}

func TestAuth__checkAuth(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	o, err := createTestOAuth()
	if err != nil {
		t.Fatal(err)
	}
	defer o.cleanup()

	repo, err := createTestUserRepository()
	if err != nil {
		t.Fatal(err)
	}
	defer repo.cleanup()

	// Setup our request
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/auth/check", nil)

	// Make HTTP request
	checkAuth(log.NewNopLogger(), auth, o.svc, repo)(w, r)
	w.Flush()

	// Since no auth information was provided we should 403
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}

	if v := w.Header().Get("X-User-Id"); v != "" {
		t.Error("expected empty X-User-Id")
	}
}

func TestAuth__forwardedPreflight(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	o, err := createTestOAuth()
	if err != nil {
		t.Fatal(err)
	}
	defer o.cleanup()

	repo, err := createTestUserRepository()
	if err != nil {
		t.Fatal(err)
	}
	defer repo.cleanup()

	// Setup our request
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/auth/check", nil)

	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("X-Forwarded-Method", "OPTIONS")

	checkAuth(log.NewNopLogger(), auth, o.svc, repo)(w, r)
	w.Flush()

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if v := w.Header().Get("Access-Control-Allow-Origin"); v != "http://localhost:8080" {
		t.Errorf("got %s", v)
	}
}
