// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
)

func main() {
	conf := oauth2.Config{
		ClientID:     "000000",
		ClientSecret: "999999",
		Scopes:       []string{"read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost:8080/authorize",
			TokenURL: "http://localhost:8080/token",
		},
	}

	// We need to pass along more fields in the token request
	u, _ := url.Parse(fmt.Sprintf("http://localhost:8080/token?grant_type=client_credentials&client_id=%s&client_secret=%s&scope=read", conf.ClientID, conf.ClientSecret))
	resp, err := http.Get(u.String())
	if err != nil {
		panic(err.Error())
	}
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err.Error())
	}

	// The fields on oauth2.Token don't all match up to the response body,
	// but we can grab the access_token and token_type fields as that turns
	// out to be all we need for successful requests.
	var token oauth2.Token
	if err := json.Unmarshal(bs, &token); err != nil {
		panic(err.Error())
	}

	client := conf.Client(context.Background(), &token)

	u, _ = url.Parse(conf.Endpoint.AuthURL)
	resp, err = client.Get(u.String())
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(resp.Status)
}
