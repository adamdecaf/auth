// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func addLoginRoutes(router *mux.Router, logger log.Logger, auth authable, userService userRepository) {
	router.Methods("GET").Path("/users/login").HandlerFunc(checkLogin(logger, auth, userService))
	router.Methods("POST").Path("/users/login").HandlerFunc(loginRoute(logger, auth, userService))
}

func getUserFromCookie(auth authable, repo userRepository, r *http.Request) (*User, error) {
	// Start checking the incoming request for cookie auth
	userId, err := extractUserId(auth, r)
	if err != nil {
		return nil, err
	}
	user, err := repo.lookupByUserId(userId)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func checkLogin(logger log.Logger, auth authable, repo userRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "checkLogin")

		if allowForOptions(w, r) {
			return // was a CORS pre-flight request
		}

		user, err := getUserFromCookie(auth, repo, r)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if user != nil {
			w.Header().Set("X-User-Id", user.ID)
		}
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(user); err != nil {
			internalError(w, err)
			return
		}
	}
}

func loginRoute(logger log.Logger, auth authable, userService userRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "loginRoute")

		if r.Body == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		bs, err := read(r.Body)
		if err != nil {
			internalError(w, err)
			return
		}

		// read request body
		var login loginRequest
		if err := json.Unmarshal(bs, &login); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Basic data sanity checks
		if err := validateEmail(login.Email); err != nil {
			moovhttp.Problem(w, err)
			return
		}
		if err := validatePassword(login.Password); err != nil {
			moovhttp.Problem(w, err)
			return
		}

		// find user by email
		u, err := userService.lookupByEmail(login.Email)
		if err != nil || u == nil {
			// Mark this (and password check) as failure only because
			// the user is involved at this point. Otherwise it's their
			// developer's problem (i.e. bad json).
			authFailures.With("method", "web").Add(1)
			w.WriteHeader(http.StatusForbidden)
			if err != nil {
				logger.Log("login", fmt.Sprintf("problem looking up user email %q: %v", login.Email, err))
			}
			return
		}

		// find user by userId and password
		if err := auth.checkPassword(u.ID, login.Password); err != nil {
			authFailures.With("method", "web").Add(1)
			logger.Log("login", fmt.Sprintf("userId=%s failed: %v", u.ID, err))
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// success route, let's finish!
		authSuccesses.With("method", "web").Add(1)
		cookie, err := createCookie(u.ID, auth)
		if err != nil {
			internalError(w, err)
			return
		}
		if cookie == nil {
			logger.Log("login", fmt.Sprintf("nil cookie for userId=%s", u.ID))
			internalError(w, err)
			return
		}
		if err := auth.writeCookie(u.ID, cookie); err != nil {
			internalError(w, err)
			return
		}

		http.SetCookie(w, cookie)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-User-Id", u.ID)
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(u); err != nil {
			internalError(w, err)
			return
		}
	}
}
