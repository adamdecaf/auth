// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

func addLogoutRoutes(router *mux.Router, logger log.Logger, auth authable) {
	router.Methods("DELETE").Path("/users/login").HandlerFunc(logoutRoute(auth))
}

func logoutRoute(auth authable) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "logoutRoute")

		userId, err := extractUserId(auth, r)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := auth.invalidateCookies(userId); err != nil {
			logger.Log("logout", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		authInactivations.With("method", "web").Add(1)
		w.WriteHeader(http.StatusOK)
	}
}
