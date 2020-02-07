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
)

func TestLogout__noCookie(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/users/logout", nil)
	logoutRoute(auth)(w, r)
	w.Flush()

	if w.Code != 200 {
		t.Errorf("got %d", w.Code)
	}
}

func TestLogout__cookieDataWithoutUser(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/users/logout", nil)
	r.Header.Set("Cookie", "random data")

	logoutRoute(auth)(w, r)
	w.Flush()

	if w.Code != 200 {
		t.Errorf("got %d", w.Code)
	}
}

func TestLogout__full(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/users/logout", nil)

	data := "user data"
	r.Header.Set("Cookie", "moov_auth="+data)

	// Write a user
	userId, _ := hash(fmt.Sprintf("%d", time.Now().Unix()))
	cookie := &http.Cookie{
		Name:    "moov_auth",
		Value:   data,
		Expires: time.Now().Add(1 * time.Hour),
	}
	if err := auth.writeCookie(userId, cookie); err != nil {
		t.Fatal(err)
	}

	// Verify userId exists
	id, err := auth.findUserId(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("no userId found")
	}

	// Perofrm logout
	logoutRoute(auth)(w, r)
	w.Flush()

	if w.Code != 200 {
		t.Errorf("got %d", w.Code)
	}

	// Check auth.findUserId
	id, err = auth.findUserId(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("userId=%s", id)
	}
}
