// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestHTTP__extractCookie(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if req == nil {
		t.Error("nil req")
	}
	req.AddCookie(&http.Cookie{
		Name:  "moov_auth",
		Value: "data",
	})

	cookie := extractCookie(req)
	if cookie == nil {
		t.Error("nil cookie")
	}
}

func TestHTTP_createCookie(t *testing.T) {
	auth, err := createTestAuthable()
	if err != nil {
		t.Fatal(err)
	}
	defer auth.cleanup()

	userId := generateID()
	cookie, err := createCookie(userId, auth)
	if err != nil {
		t.Fatal(err)
	}

	if id, _ := auth.findUserId(cookie.Value); id != userId {
		t.Errorf("got %q, expected %q", id, userId)
	}
}

func TestHTTP_internalError(t *testing.T) {
	w := httptest.NewRecorder()
	internalError(w, errors.New("problem Y"))
	w.Flush()

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestHttp__addPingRoute(t *testing.T) {
	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	addPingRoute(router)
	router.ServeHTTP(w, r)
	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if v := w.Body.String(); v != "PONG" {
		t.Errorf("got %q", v)
	}
}
