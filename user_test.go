// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

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

// createTestAuthable returns a new 'auth' instance
// wrapped with a cleanup() method.
//
// path is optional, if empty a new ioutil.TempDir will be
// created and optionally cleaned up.
func createTestAuthable() (*testAuth, error) {
	dir, err := ioutil.TempDir("", "auth")
	if err != nil {
		return nil, err
	}

	db, err := createSqliteConnection(filepath.Join(dir, "auth.db"))
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

// createTestUserRepository
func createTestUserRepository() (*testUserRepository, error) {
	dir, err := ioutil.TempDir("", "userRepository")
	if err != nil {
		return nil, err
	}

	db, err := createSqliteConnection(filepath.Join(dir, "auth.db"))
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
