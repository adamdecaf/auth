// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	stderr "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/moov-io/auth/pkg/buntdbclient"
	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"gopkg.in/oauth2.v3"
	"gopkg.in/oauth2.v3/errors"
	"gopkg.in/oauth2.v3/manage"
	"gopkg.in/oauth2.v3/models"
	"gopkg.in/oauth2.v3/server"
	"gopkg.in/oauth2.v3/store"
)

var (
	errNoClientId = stderr.New("missing client_id")
)

type oauth struct {
	manager     *manage.Manager
	clientStore *buntdbclient.ClientStore
	server      *server.Server

	logger log.Logger
}

func setupOAuthTokenStore(path string) (oauth2.TokenStore, error) {
	if path == "" {
		path = "oauth2_tokens.db"
	}
	tokenStore, err := store.NewFileTokenStore(path)
	if err != nil {
		return nil, fmt.Errorf("problem creating token store: %v", err)
	}
	return tokenStore, nil
}

func setupOAuthClientStore(path string) (*buntdbclient.ClientStore, error) {
	if path == "" {
		path = "oauth2_clients.db"
	}
	cs, err := buntdbclient.New(path)
	if err != nil {
		return nil, fmt.Errorf("problem creating clients store: %v", err)
	}
	return cs, nil
}

func setupOAuthServer(logger log.Logger, tokenStore oauth2.TokenStore, clientStore *buntdbclient.ClientStore) (*oauth, error) {
	out := &oauth{
		logger: logger,
	}

	// Create our session manager
	out.manager = manage.NewDefaultManager()
	out.manager.MapTokenStorage(tokenStore)

	// Defaults from (in vendor/)
	// gopkg.in/oauth2.v3/manage/config.go
	cfg := &manage.Config{
		AccessTokenExp:    2 * time.Hour,
		RefreshTokenExp:   24 * 3 * time.Hour,
		IsGenerateRefresh: true,
	}
	out.manager.SetAuthorizeCodeTokenCfg(cfg)
	out.manager.SetClientTokenCfg(cfg)

	// Setup oauth2 clients database
	out.clientStore = clientStore
	out.manager.MapClientStorage(out.clientStore)

	out.server = server.NewDefaultServer(out.manager)
	out.server.SetAllowGetAccessRequest(true)
	out.server.SetClientInfoHandler(server.ClientFormHandler)
	out.server.SetInternalErrorHandler(func(err error) (re *errors.Response) {
		logger.Log("internal-error", err.Error())
		return
	})
	out.server.SetResponseErrorHandler(func(re *errors.Response) {
		m := re.Error.Error()
		if m == "server_error" || m == "unsupported_grant_type" {
			return
		}
		logger.Log("response-error", m)
	})

	return out, nil
}

// addOAuthRoutes includes our oauth2 routes on the provided mux.Router
func addOAuthRoutes(r *mux.Router, o *oauth, logger log.Logger, auth authable) {
	r.Methods("GET").Path("/oauth2/authorize").HandlerFunc(o.authorizeHandler)
	r.Methods("POST").Path("/oauth2/client").HandlerFunc(o.createClientHandler(auth))

	// Check token routes
	if o.server.Config.AllowGetAccessRequest {
		// only open up GET if the server config asks for it
		r.Methods("GET").Path("/oauth2/token").HandlerFunc(o.tokenHandler)
	}
	r.Methods("POST").Path("/oauth2/token").HandlerFunc(o.tokenHandler)
}

// requestHasValidOAuthToken hooks into the go-oauth2 methods to validate
// a 'Bearer ...' Authorization header and the token.
func (o *oauth) requestHasValidOAuthToken(r *http.Request) (oauth2.TokenInfo, error) {
	// We aren't using HandleAuthorizeRequest here because that assumes redirect_uri
	// exists on the request. We're just checking for a valid token.
	ti, err := o.server.ValidationBearerToken(r)
	if err != nil {
		authFailures.With("method", "oauth2").Add(1)
		return nil, err
	}
	if ti.GetClientID() == "" {
		authFailures.With("method", "oauth2").Add(1)
		return nil, errNoClientId
	}
	return ti, nil
}

// authorizeHandler checks the request for appropriate oauth information
// and returns "200 OK" if the token is valid.
func (o *oauth) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	w = wrapResponseWriter(w, r, "oauth.authorizeHandler")

	if _, err := o.requestHasValidOAuthToken(r); err != nil {
		moovhttp.Problem(w, err)
		return
	}

	// Passed token check, return "200 OK"
	authSuccesses.With("method", "oauth2").Add(1)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

// tokenHandler passes off the request down to our oauth2 library to
// generate a token (or return an error).
func (o *oauth) tokenHandler(w http.ResponseWriter, r *http.Request) {
	w = wrapResponseWriter(w, r, "oauth.tokenHandler")

	// This block is copied from o.server.HandleTokenRequest
	// We needed to inspect what's going on a bit.
	gt, tgr, verr := o.server.ValidationTokenRequest(r)
	if verr != nil {
		moovhttp.Problem(w, verr)
		return
	}
	ti, verr := o.server.GetAccessToken(gt, tgr)
	if verr != nil {
		moovhttp.Problem(w, verr)
		return
	}
	data := o.server.GetTokenData(ti)
	bs, err := json.Marshal(data)
	if err != nil {
		moovhttp.Problem(w, err)
		return
	}
	// (end of copy)

	// HandleTokenRequest currently returns nil even if the token request
	// failed. That menas we can't clearly know if token generation passed or failed.
	// We check ww.Code then, it'll be 0 if no WriteHeader calls were made.
	if ww, ok := w.(*httptest.ResponseRecorder); ok && ww.Code == 0 {
		tokenGenerations.Add(1)
		w.Header().Set("X-User-Id", ti.GetUserID()) // only on non-errors
	}

	// Write our response
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(bs)
}

// createClientHandler will create an oauth client for the authenticated user.
//
// This method extracts the user from the cookies in r.
func (o *oauth) createClientHandler(auth authable) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "oauth.createTokenHandler")

		userId, err := auth.findUserId(extractCookie(r).Value)
		if err != nil {
			// user not found, return
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// TODO(adam): don't create tokens if user hasn't gone through email verification

		records, err := o.clientStore.GetByUserID(userId)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			internalError(w, err)
			return
		}
		if len(records) == 0 { // nothing found, so fake one
			records = append(records, &models.Client{})
		}

		clients := make([]*models.Client, len(records))
		for i := range records {
			err = o.clientStore.DeleteByID(records[i].GetID())
			if err != nil && !strings.Contains(err.Error(), "not found") {
				internalError(w, err)
				return
			}

			clients[i] = &models.Client{
				ID:     generateID()[:12],
				Secret: generateID(),
				Domain: Domain,
				UserID: userId,
			}

			// Write client into oauth clients db.
			if err := o.clientStore.Set(clients[i].GetID(), clients[i]); err != nil {
				internalError(w, err)
				return
			}
		}

		// metrics
		clientGenerations.Add(1)

		// render back new clients
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		var responseClients []*client
		for i := range clients {
			responseClients = append(responseClients, &client{
				ClientID:     clients[i].ID,
				ClientSecret: clients[i].Secret,
				Domain:       clients[i].Domain,
			})
		}
		if err := json.NewEncoder(w).Encode(responseClients); err != nil {
			internalError(w, err)
			return
		}
	}
}

type client struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Domain       string `json:"domain"`
}

func (o *oauth) shutdown() error {
	if o == nil || o.clientStore == nil {
		return nil
	}
	return o.clientStore.Close()
}
