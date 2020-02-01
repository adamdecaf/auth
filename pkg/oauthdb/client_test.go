// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package oauthdb

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"gopkg.in/oauth2.v3/models"
)

type testClientStore struct {
	*ClientStore
}

func (cs *testClientStore) Close() error {
	return cs.ClientStore.Close()
}

func createTestClientStore() (*testClientStore, error) {
	dir, err := ioutil.TempDir("", "oauthdb-client")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "oauth_clients.db")

	cs, err := NewClientStoreDB(fmt.Sprintf("file:%s", path))
	if err != nil {
		return nil, err
	}

	return &testClientStore{
		ClientStore: cs,
	}, nil
}

func TestClientStore(t *testing.T) {
	cs, err := createTestClientStore()
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// expect nothing
	client, err := cs.GetByID(generateID())
	if err != nil {
		t.Error("expected no error (record not found)")
	}
	if client != nil {
		t.Errorf("got unexpected client: %v", client)
	}

	// write something
	userId := generateID()
	c := &models.Client{
		ID:     generateID(),
		Secret: generateID(),
		Domain: "api.moov.io",
		UserID: userId,
	}
	if err := cs.Set(c.ID, c); err != nil {
		t.Fatalf("problem writing %v: %v", c, err)
	}

	// read something back
	client, err = cs.GetByID(c.ID)
	if err != nil {
		t.Fatalf("problem reading client %s: %v", c.ID, err)
	}
	if client == nil {
		t.Fatal("expected client, but got none")
	}

	// get by UserId
	clients, err := cs.GetByUserID(userId)
	if err != nil {
		t.Error("expected no error (record not found)")
	}
	if len(clients) != 1 {
		t.Errorf("got unexpected clients: %v", clients)
	}

	// delete and verify it's gone
	if err := cs.DeleteByID(c.ID); err != nil {
		t.Fatalf("problem deleting client: %v", err)
	}

	// confirm it's gone
	client, err = cs.GetByID(c.ID)
	if err != nil || client != nil {
		t.Fatalf("expected nothing, but got client=%v err=%v", client, err)
	}
}
