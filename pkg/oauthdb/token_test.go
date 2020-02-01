// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package oauthdb

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/oauth2.v3/models"
)

type testTokenStore struct {
	*TokenStore
}

func (ts *testTokenStore) Close() error {
	return ts.TokenStore.Close()
}

func createTestTokenStore() (*testTokenStore, error) {
	dir, err := ioutil.TempDir("", "oauthdb-token")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "oauth2_tokens.db")

	ts, err := NewTokenStoreDB(fmt.Sprintf("file:%s", path))
	if err != nil {
		return nil, err
	}

	return &testTokenStore{
		TokenStore: ts,
	}, nil
}

func TestTokenStore__Create(t *testing.T) {
	ts, err := createTestTokenStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	// write something
	userId := generateID()
	tk := &models.Token{
		ClientID:        generateID(),
		UserID:          userId,
		Access:          generateID(),
		AccessCreateAt:  time.Now().Add(-1 * time.Second), // in the past
		AccessExpiresIn: 30 * time.Minute,                 // the future
	}
	if err := ts.Create(tk); err != nil {
		t.Fatal(err)
	}

	// read something
	token, err := ts.GetByAccess(tk.Access)
	if err != nil || token == nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// Ok, cool - let's change the UserId now and watch .Create update that
	tk.SetUserID(generateID())
	if err := ts.Create(tk); err != nil {
		t.Fatal(err)
	}

	// Read that token back and match userIds
	token2, err := ts.GetByAccess(tk.Access)
	if err != nil || token2 == nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token2, err)
	}
	if token2.GetUserID() != tk.GetUserID() {
		t.Fatalf("expected userId to update, but didn't")
	}
}

func TestTokenStore__ByAccess(t *testing.T) {
	ts, err := createTestTokenStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	// expect nothing
	token, err := ts.GetByAccess(generateID())
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// write something
	userId := generateID()
	tk := &models.Token{
		ClientID:        generateID(),
		UserID:          userId,
		Access:          generateID(),
		AccessCreateAt:  time.Now().Add(-1 * time.Second), // in the past
		AccessExpiresIn: 30 * time.Minute,                 // the future
	}
	if err := ts.Create(tk); err != nil {
		t.Fatal(err)
	}

	// read something
	token, err = ts.GetByAccess(tk.Access)
	if err != nil || token == nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// delete something
	if err := ts.RemoveByAccess(tk.Access); err != nil {
		t.Fatal(err)
	}

	// read nothing
	token, err = ts.GetByAccess(tk.Access)
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}
}

func TestTokenStore__ByCode(t *testing.T) {
	ts, err := createTestTokenStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	// expect nothing
	token, err := ts.GetByCode(generateID())
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// write something
	userId := generateID()
	tk := &models.Token{
		ClientID:      generateID(),
		UserID:        userId,
		Code:          generateID(),
		CodeCreateAt:  time.Now().Add(-1 * time.Second), // in the past
		CodeExpiresIn: 30 * time.Minute,                 // the future
	}
	if err := ts.Create(tk); err != nil {
		t.Fatal(err)
	}

	// read something
	token, err = ts.GetByCode(tk.Code)
	if err != nil || token == nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// delete something
	if err := ts.RemoveByCode(tk.Code); err != nil {
		t.Fatal(err)
	}

	// read nothing
	token, err = ts.GetByCode(tk.Code)
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}
}

func TestTokenStore__ByRefresh(t *testing.T) {
	ts, err := createTestTokenStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	// expect nothing
	token, err := ts.GetByRefresh(generateID())
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// write something
	userId := generateID()
	tk := &models.Token{
		ClientID:         generateID(),
		UserID:           userId,
		Refresh:          generateID(),
		RefreshCreateAt:  time.Now().Add(-1 * time.Second), // in the past
		RefreshExpiresIn: 30 * time.Minute,                 // the future
	}
	if err := ts.Create(tk); err != nil {
		t.Fatal(err)
	}

	// read something
	token, err = ts.GetByRefresh(tk.Refresh)
	if err != nil || token == nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}

	// delete something
	if err := ts.RemoveByRefresh(tk.Refresh); err != nil {
		t.Fatal(err)
	}

	// read nothing
	token, err = ts.GetByRefresh(tk.Refresh)
	if err != nil || token != nil {
		t.Fatalf("expected nothing, but got token=%v err=%v", token, err)
	}
}
