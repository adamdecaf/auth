// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

var (
	errNoCookie     = fmt.Errorf("no %s cookie provided", cookieName)
	errUserNotFound = errors.New("user not found")
)

// extractUserId tries to find the Moov userId associated to the cookie data.
// This function will never return a blank userId with a nil error.
func extractUserId(auth authable, r *http.Request) (string, error) {
	cookie := extractCookie(r)
	if cookie == nil {
		return "", errNoCookie
	}
	userId, err := auth.findUserId(cookie.Value)
	if err != nil {
		return "", errUserNotFound
	}
	if userId == "" {
		return "", errUserNotFound
	}
	return userId, nil
}

func addAuthRoutes(router *mux.Router, logger log.Logger, auth authable, o *oauth, repo userRepository) {
	router.Methods("GET").Path("/auth/check").HandlerFunc(checkAuth(logger, auth, o, repo))
}

func checkAuth(logger log.Logger, auth authable, o *oauth, repo userRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "checkAuth")

		if allowForOptions(w, r) {
			return // was a CORS pre-flight request
		}

		user, _ := getUserFromCookie(auth, repo, r)
		token, err := o.requestHasValidOAuthToken(r)

		if user == nil && err != nil { // no user from cookie and no oauth credentials
			w.WriteHeader(http.StatusForbidden)
			return
		}

		var userId string
		if user != nil && user.ID != "" {
			userId = user.ID
		}
		if token != nil && userId == "" {
			userId = token.GetUserID()
		}

		if userId == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.Header().Set("X-User-Id", userId)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}
}
