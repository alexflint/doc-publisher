package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/alexflint/doc-publisher/ui"
	"github.com/alexflint/go-arg"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

//go:embed secrets/storage_service_account.json
var storageServiceAccount []byte

//go:embed secrets/lesswrong-password.txt
var lesswrongPassword string

//go:embed cards/root.json
var rootCard []byte

//go:embed cards/demo.json
var demoCard []byte

//go:embed cards/requestAccess.json
var requestAccess []byte

//go:embed cards/accessGranted.json
var accessGranted []byte

//go:embed cards/textChanged.json
var textChanged []byte

// workspacePayload models the JSON payload sent to our endpoints by Google Workspaces
type workspacePayload struct {
	Authorization workspaceAuthorization `json:"authorizationEventObject"`
	Common        commonEventObject      `json:"commonEventObject"`
}

// workspaceAuthorization is how Google Workspaces gives us the authentication tokens
type workspaceAuthorization struct {
	UserIdToken    string           `json:"userIdToken"`
	UserOAuthToken string           `json:"userOAuthToken"`
	Docs           *docsEventObject `json:"docs"`
}

// docsEventObject identifies the Google Doc that the user is working with
// See https://developers.google.com/apps-script/add-ons/concepts/event-objects#docs_event_object
type docsEventObject struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	HasFileScope bool   `json:"addonHasFileScopePermission"`
}

// commonEventObject identifies the host, the platform, and any form inputs provided by the user
type commonEventObject struct {
	HostApp    string               `json:"hostApp"`  // "DOCS", "GMAIL", "CALENDAR"
	Platform   string               `json:"platform"` // "WEB"
	FormInputs map[string]formInput // Input name -> value
}

// formInput models the data that Workspaces sends us when a form is submitted. There
// is one of these for each widget.
type formInput struct {
	StringInputs stringInputs `json:"stringInputs"`
}

// stringInputs models the data the Workspaces sends us for a textbox when a form is
// submitted. I don't know why "value" is an array or in what cases it would have other
// than one element.
type stringInputs struct {
	Value []string `json:"value"`
}

// invoked when the user clicks "publish"
func handlePublish(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Err(err).Msg("error reading request body")
		return
	}

	log.Print("received at root: ", string(body))

	var payload workspacePayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		http.Error(w, "error parsing body as JSON", http.StatusBadRequest)
		log.Err(err).Msg("error parsing body as JSON")
		return
	}

	// get the lesswrong ID for this post
	//   get the user ID
	//   get the document ID
	//   form a key
	//   look up record in datastore
	// create or update the lesswrong post

	creds := google.Credentials{
		ProjectID: "doc-publisher-341418",
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: payload.Authorization.UserOAuthToken,
		}),
	}

	// get the document ID
	if payload.Authorization.Docs == nil {
		log.Print("no document ID was provided")
		http.Error(w, "no document ID was provided", http.StatusBadRequest)
		return
	}
	docID := payload.Authorization.Docs.ID
	if docID == "" {
		log.Print("document ID was provided but was empty")
		http.Error(w, "document ID was provided but was empty", http.StatusBadRequest)
		return
	}

	// publish the document
	result, err := publish(r.Context(), &creds, docID, "")
	if err != nil {
		log.Err(err).Msg("error publishing document to lesswrong")
		http.Error(w, "error publishing document: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// tell the addon UI to open the new lesswrong post in the user's browser
	response := ui.Response{
		RenderActions: &ui.RenderActions{
			Action: &ui.Action{
				Link: &ui.OpenLink{
					URL: result.URL,
				},
			},
		},
	}

	// success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// invoked when the user edits the "document ID" textbox in the root card
// renders nothing
func handleTextChanged(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	var payload workspacePayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		http.Error(w, "error parsing body as JSON", http.StatusBadRequest)
		log.Printf("error parsing body as JSON: %v", err)
		return
	}

	if x, ok := payload.Common.FormInputs["Document ID"]; ok && len(x.StringInputs.Value) == 1 {
		docID := x.StringInputs.Value[0]
		_ = docID
	}

	log.Print("received at textChanged: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(textChanged)
}

// invoked when the user clicks "authorize" on the root card
// renders a card that asks the user to give this addon access to the google doc they have open
func handleRequestAccess(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Print("received at requestAccess: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(requestAccess)
}

// invoked when the user grants this addon access to the google doc they have open
// renders an empty card
func handleAccessGranted(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Print("received at accessGranted: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(accessGranted)
}

func handleDemo(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Print("received at demo: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(demoCard)
}

// handleRoot is called when the addon is initialized. It renders the top-level card.
func handleRoot(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Print("received at root: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(rootCard)
}

func main() {
	var args struct {
		PrettyLogs bool `help:"human-readable logs for local testing"`
		Port       int  `arg:"positional,env:PORT" default:"8000"` // this will not contain a leading colon due to Cloud Run API
	}
	arg.MustParse(&args)

	if args.PrettyLogs {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	http.Handle("/textChanged", http.HandlerFunc(handleTextChanged))
	http.Handle("/accessGranted", http.HandlerFunc(handleAccessGranted))
	http.Handle("/requestAccess", http.HandlerFunc(handleRequestAccess))
	http.Handle("/demo", http.HandlerFunc(handleDemo))
	http.Handle("/publish", http.HandlerFunc(handlePublish))
	http.Handle("/", http.HandlerFunc(handleRoot))

	// we must add the colon ourselves because Cloud Run will give us an integer port
	port := fmt.Sprintf(":%d", args.Port)
	log.Print("listening on " + port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Err(err).Msg("http.ListenAndServe returned with error")
	}
}
