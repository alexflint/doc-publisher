package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/alexflint/doc-publisher/googledoc"
	"github.com/alexflint/doc-publisher/lesswrong"
	"github.com/alexflint/doc-publisher/markdown"
	"github.com/alexflint/go-arg"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
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

// represents the JSON payload sent to us by Google Workspace
type workspaceAuthorization struct {
	UserIdToken    string            `json:"userIdToken"`
	UserOAuthToken string            `json:"userOAuthToken"`
	Docs           *docsEventObject  `json:"docs"`
	Drive          *driveEventObject `json:"drive"`
}

// Info about the active document, sent to us by Google Workspace
// See https://developers.google.com/apps-script/add-ons/concepts/event-objects#docs_event_object
type docsEventObject struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	HasFileScope bool   `json:"addonHasFileScopePermission"`
}

// Info about the active file
// See https://developers.google.com/apps-script/add-ons/concepts/event-objects#docs_event_object
type driveEventObject struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	HasFileScope bool   `json:"addonHasFileScopePermission"`
}

type workspacePayload struct {
	Authorization workspaceAuthorization `json:"authorizationEventObject"`
	Common        commonEventObject      `json:"commonEventObject"`
}

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
		log.Println("##### textChanged received document ID: " + docID)
	}

	log.Println("received at textChanged: ", string(body))

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

	log.Println("received at requestAccess: ", string(body))

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

	log.Println("received at accessGranted: ", string(body))

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

	log.Println("received at demo: ", string(body))

	w.Header().Set("Content-Type", "application/json")
	w.Write(demoCard)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Println("received at root: ", string(body))

	var payload workspacePayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		http.Error(w, "error parsing body as JSON", http.StatusBadRequest)
		log.Printf("error parsing body as JSON: %v", err)
		return
	}

	err = tryStuff(r.Context(), &payload)
	if err != nil {
		log.Println(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rootCard)
}

func tryStuff(ctx context.Context, payload *workspacePayload) error {
	creds := google.Credentials{
		ProjectID: "doc-publisher-341418",
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: payload.Authorization.UserOAuthToken,
		}),
	}

	docID := "1Wsar2ajCKHBA8OlSPUe1SnPWg7kalV-4AfK-Hrk_rN8"

	// create the docs client
	docsClient, err := docs.NewService(ctx,
		option.WithCredentials(&creds),
		option.WithScopes(docs.DriveFileScope))
	if err != nil {
		return fmt.Errorf("error creating docs client")
	}

	// create the drive client
	driveClient, err := drive.NewService(ctx,
		option.WithCredentials(&creds),
		option.WithScopes(docs.DriveFileScope))
	if err != nil {
		return fmt.Errorf("error creating drive client")
	}

	// fetch the google doc
	d, err := googledoc.Fetch(ctx, docID, docsClient, driveClient)
	if err != nil {
		return fmt.Errorf("error fetching google doc: %w", err)
	}

	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx,
		option.WithCredentialsJSON(storageServiceAccount))
	if err != nil {
		return fmt.Errorf("error creating cloud storage client")
	}

	// upload images
	imageURLs, err := googledoc.UploadImages(ctx, d.Images, storageClient.Bucket("doc-publisher-images"))
	if err != nil {
		return fmt.Errorf("error uploading images: %w", err)
	}

	// match image URLs to object IDs
	imageURLsByObjectID, err := googledoc.MatchObjectIDsToImages(d, imageURLs)
	if err != nil {
		return fmt.Errorf("error matching image URLs to object IDs: %w", err)
	}

	// convert to markdown
	md, err := markdown.FromGoogleDoc(d.Doc, imageURLsByObjectID)
	if err != nil {
		return fmt.Errorf("error converting google doc to markdown: %w", err)
	}

	// create the lesswrong client
	lw, err := lesswrong.NewClient(ctx, "alex.flint@gmail.com", lesswrongPassword)
	if err != nil {
		return fmt.Errorf("error authenticating with lesswrong: %w", err)
	}

	// create the post
	resp, err := lw.CreatePost(ctx, lesswrong.CreatePostRequest{
		Title:   "brand new post",
		Content: md,
	})
	if err != nil {
		return fmt.Errorf("error creating lesswrong post: %w", err)
	}

	log.Println("created lesswrong post: " + resp.URL)

	return nil
}

func main() {
	var args struct {
		Port int `arg:"positional,env:PORT" default:"8000"` // this will not contain a leading colon due to Cloud Run API
	}
	arg.MustParse(&args)

	http.Handle("/textChanged", http.HandlerFunc(handleTextChanged))
	http.Handle("/accessGranted", http.HandlerFunc(handleAccessGranted))
	http.Handle("/requestAccess", http.HandlerFunc(handleRequestAccess))
	http.Handle("/demo", http.HandlerFunc(handleDemo))
	http.Handle("/", http.HandlerFunc(handleRoot))

	// we must add the colon ourselves because Cloud Run will give us an integer port
	port := fmt.Sprintf(":%d", args.Port)
	log.Println("listening on " + port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Println(err)
	}
}
