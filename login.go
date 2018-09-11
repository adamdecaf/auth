// Copyright 2018 The ACH Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

var (
	errUserNotFound = errors.New("user not found")
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func addLoginRoutes(router *mux.Router, logger log.Logger, auth authable, userService userRepository) {
	router.Methods("POST").Path("/users/login").HandlerFunc(loginRoute(logger, auth, userService))
}

func loginRoute(logger log.Logger, auth authable, userService userRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		bs, err := read(r.Body)
		if err != nil {
			internalError(w, err, "login")
			return
		}

		// read request body
		var login loginRequest
		if err := json.Unmarshal(bs, &login); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logger.Log("login", err)
			return
		}

		// find user by email
		u, err := userService.lookupByEmail(login.Email)
		if err != nil {
			// Mark this (and password check) as failure only because
			// the user is involved at this point. Otherwise it's their
			// developer's problem (i.e. bad json).
			authFailures.With("method", "web").Add(1)
			encodeError(w, errUserNotFound)
		}

		// find user by userId and password
		if err := auth.checkPassword(u.ID, login.Password); err != nil {
			authFailures.With("method", "web").Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		} else {
			authSuccesses.With("method", "web").Add(1)
			w.WriteHeader(http.StatusOK)
			// TODO(adam): return user json
			// TODO(adam): set cookie
		}
	}
}
