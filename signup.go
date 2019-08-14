// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moov-io/base"
	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

const (
	minPasswordLength = 8
)

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`

	// misc profile information
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Phone      string `json:"phone"`
	CompanyURL string `json:"companyUrl,omitempty"`
}

func addSignupRoutes(router *mux.Router, logger log.Logger, auth authable, userService userRepository) {
	router.Methods("POST").Path("/users/create").HandlerFunc(signupRoute(auth, userService))
}

func signupRoute(auth authable, userService userRepository) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w = wrapResponseWriter(w, r, "signupRoute")

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
		var signup signupRequest
		if err := json.Unmarshal(bs, &signup); err != nil {
			logger.Log("signup", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		requestID := moovhttp.GetRequestID(r)

		// Basic data sanity checks
		if err := validateEmail(signup.Email); err != nil {
			moovhttp.Problem(w, err)
			if requestID != "" && logger != nil {
				logger.Log("signup", fmt.Sprintf("(requestID=%s) invalid email: %v", requestID, err))
			}
			return
		}
		if err := validatePassword(signup.Password); err != nil {
			moovhttp.Problem(w, err)
			if requestID != "" && logger != nil {
				logger.Log("signup", fmt.Sprintf("(requestID=%s) invalid password: %v", requestID, err))
			}
			return
		}
		if err := validatePhone(signup.Phone); err != nil {
			moovhttp.Problem(w, err)
			if requestID != "" && logger != nil {
				logger.Log("signup", fmt.Sprintf("(requestID=%s) invalid phone number: %v", requestID, err))
			}
			return
		}

		// find user
		u, err := userService.lookupByEmail(signup.Email)
		if err != nil {
			internalError(w, fmt.Errorf("problem looking up user email %q: %v", signup.Email, err))
			return
		}
		if u == nil {
			var signup signupRequest
			if err := json.Unmarshal(bs, &signup); err != nil {
				moovhttp.Problem(w, err)
				logger.Log("signup", fmt.Sprintf("failed parsing request json: %v", err))
				return
			}

			// store user
			userId := generateID()
			if userId == "" {
				internalError(w, fmt.Errorf("problem creating userId, err=%v", err))
				return
			}
			u = &User{
				ID:         userId,
				Email:      signup.Email,
				FirstName:  signup.FirstName,
				LastName:   signup.LastName,
				Phone:      signup.Phone,
				CompanyURL: signup.CompanyURL,
				CreatedAt:  base.NewTime(time.Now()),
			}
			if err := userService.upsert(u); err != nil {
				internalError(w, fmt.Errorf("problem writing user: %v", err))
				return
			}

			if err := auth.writePassword(u.ID, signup.Password); err != nil {
				internalError(w, fmt.Errorf("problem writing user credentials: %v", err))
				return
			}

			// signup worked, yay!
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))

			// TODO(adam): email approval link and clickthrough
		} else {
			// user found, so reject signup
			moovhttp.Problem(w, fmt.Errorf("user already exists - %s", signup.Email))
		}
	}
}

func validateEmail(email string) error {
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("no email provided")
	}
	return nil
}

func validatePassword(pass string) error {
	if pass == "" {
		return errors.New("no password provided")
	}
	if n := utf8.RuneCountInString(pass); n < minPasswordLength {
		return fmt.Errorf("password required to be at least %d characters", minPasswordLength)
	}
	return nil
}

func validatePhone(phone string) error {
	phone = strings.Replace(phone, "-", "", -1)
	phone = strings.Replace(phone, ".", "", -1)
	phone = strings.Replace(phone, " ", "", -1)
	if m, _ := regexp.MatchString("^\\+?[1-9]\\d{1,14}$", phone); !m {
		return errors.New("phone number is invalid")
	}
	return nil
}
