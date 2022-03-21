package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

//go:embed secrets/oauth_credentials.json
var oauthCredentials []byte

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	var tok oauth2.Token
	err = json.NewDecoder(f).Decode(&tok)
	if err != nil {
		return nil, err
	}

	return &tok, nil
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer f.Close()
	if err != nil {
		return err
	}

	return json.NewEncoder(f).Encode(token)
}

type googleTokenSource struct {
	ctx     context.Context
	config  *oauth2.Config
	tokFile string
}

func (ts googleTokenSource) Token() (*oauth2.Token, error) {
	authURL := ts.config.AuthCodeURL("state-token") //, oauth2.AccessTypeOffline)
	err := browser.OpenURL(authURL)
	if err != nil {
		fmt.Println("Go to the following link in your browser then type the " +
			"authorization code:\n" + authURL)
	}

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := ts.config.Exchange(ts.ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token from web: %w", err)
	}

	if ts.tokFile != "" {
		fmt.Printf("storing token to %s\n", ts.tokFile)
		err = saveToken(ts.tokFile, tok)
		if err != nil {
			return nil, fmt.Errorf("error saving token to file: %w", err)
		}
	}

	return tok, nil
}

// GoogleAuth authenticates with Google using oauth
func GoogleAuth(ctx context.Context, tokFile string, scopes ...string) (oauth2.TokenSource, error) {
	config, err := google.ConfigFromJSON(oauthCredentials, scopes...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	// create a token source that performs the oauth flow
	var ts oauth2.TokenSource = googleTokenSource{
		ctx:     ctx,
		config:  config,
		tokFile: tokFile,
	}

	// try load the token from a file
	tok, err := tokenFromFile(tokFile)
	fmt.Printf("looking for token at %s\n", tokFile)
	if err == nil {
		ts = oauth2.ReuseTokenSource(tok, ts)
	}

	return ts, nil
}
