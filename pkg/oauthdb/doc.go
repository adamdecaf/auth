// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

// Package oauthdb implements ClientStore and TokenStore from gopkg.in/oauth2.v3
// using Go's sql.DB
//
// The implementation is tested with SQLite, but should work with other SQL engines
// as we attempt to use platform agnostic SQL queries.
//
// A few extra operations on ClientStore have been added though, such as Set and
// GetByUserId. These were needed for ourusecase as we're mutating
// the oauth clients.
package oauthdb
