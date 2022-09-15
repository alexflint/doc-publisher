package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

//go:embed secrets/oauth.json
var oauthJSON []byte

// Read a token from a local file.
func readToken(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

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
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

type googleTokenSource struct {
	ctx     context.Context
	config  *oauth2.Config
	tokFile string
}

func (ts googleTokenSource) Token() (*oauth2.Token, error) {
	// pick an unused port to listen on
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("error opening a TCP port to receive the oauth callback")
	}

	// the http server below will populate this with the authentication code from the callback
	var authCode string

	// set up HTTP server to listen for the callback from Google
	var server http.Server
	server = http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Printf("oauth server received %s request for %s\n", r.Method, r.URL.Path)
			if r.URL.Path != "/" {
				fmt.Printf("oauth http server received a request for path %s, ignoring\n", r.URL.Path)
				return
			}

			query := r.URL.Query()
			authCode = query.Get("code")
			fmt.Fprintf(w, "doc-publisher is authenticated. You may now close this page and return to the terminal.")

			// now gracefully shutdown the server -- server.Serve below will return immediately
			go server.Shutdown(context.Background())
		}),
	}

	// open the user's browser to the oauth screen
	ts.config.RedirectURL = fmt.Sprintf("http://localhost:%d/", listener.Addr().(*net.TCPAddr).Port)
	authURL := ts.config.AuthCodeURL("state-token")
	err = browser.OpenURL(authURL)
	fmt.Println(authURL)
	if err != nil {
		fmt.Println("Go to the following link in your browser:\n" + authURL)
	}

	// run the HTTP server and wait for the callback
	err = server.Serve(listener)
	fmt.Println("server.Serve returned with err=", err)
	if err != nil && err != http.ErrServerClosed {
		return nil, fmt.Errorf("error running HTTP server to get oauth callback: %v", err)
	}

	// check that we got an auth code
	if authCode == "" {
		return nil, errors.New("there was no auth code in the callback from oauth flow")
	}

	// use the auth code to get a token
	tok, err := ts.config.Exchange(ts.ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token from web: %w", err)
	}

	// save the token to a file so that next time we might not have to go through the flow
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
	config, err := google.ConfigFromJSON(oauthJSON, scopes...)
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
	tok, err := readToken(tokFile)
	fmt.Printf("looking for token at %s\n", tokFile)
	if err == nil {
		fmt.Printf("reusing token from %s\n", tokFile)
		ts = oauth2.ReuseTokenSource(tok, ts)
	}

	return ts, nil
}
