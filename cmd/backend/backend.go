package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v2"
	"google.golang.org/api/option"
)

const resp = `
{
	"action": {
		"navigations": [{
			"pushCard": {
				"sections": [{
					"widgets": [
						{
							"textParagraph": {
								"text": "Hello world!"
							}
						}
					]
				}],
				"fixedFooter": {
					"primaryButton": {
						"text": "Authorize file access",
						"onClick": {
							"action": {
								"function": "https://doc-publisher-backend-mxh6l73c4a-uc.a.run.app/authorizeFile"
							}
						}
					}
				}
			}
		}]
	}
}`

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
}

func handleAuthorizeFile(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{
		"renderActions": {
			"hostAppAction": {
				"editorAction": {
					"requestFileScopeForActiveDocument": {}
				}
			}
		}
	}`))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("error reading request body: %v", err), http.StatusInternalServerError)
		log.Printf("error reading request body: %v", err)
		return
	}

	log.Println(string(body))

	var payload workspacePayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing body: %v", err), http.StatusBadRequest)
		log.Printf("error parsing body: %v", err)
		return
	}

	err = tryStuff(r.Context(), &payload)
	if err != nil {
		log.Println(err)
	}

	http.ServeContent(w, r, "response.json", time.Time{}, strings.NewReader(resp))
}

func tryStuff(ctx context.Context, payload *workspacePayload) error {
	creds := google.Credentials{
		ProjectID: "doc-publisher-341418",
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: payload.Authorization.UserOAuthToken,
		}),
	}

	docID := "1Wsar2ajCKHBA8OlSPUe1SnPWg7kalV-4AfK-Hrk_rN8"

	// create the drive client
	driveClient, err := drive.NewService(ctx,
		//option.WithTokenSource(creds.TokenSource),
		option.WithCredentials(&creds),
		option.WithScopes(docs.DriveFileScope))

	if err != nil {
		return fmt.Errorf("error creating drive client: %w", err)
	}

	// export the document as a zip arcive
	log.Printf("attempting to export %q", docID)
	_, err = driveClient.Files.Export(docID, "application/zip").Download()
	if err == nil {
		log.Println("exported the zip file!")
	} else {
		log.Println("error exporting zip file: ", err)
	}

	// create the docs client
	docsClient, err := docs.NewService(ctx,
		//option.WithTokenSource(creds.TokenSource),
		option.WithCredentials(&creds),
		option.WithScopes(docs.DriveFileScope))
	if err != nil {
		log.Fatalln("error creating docs client: ", err)
	}

	// fetch the document
	log.Printf("attempting to fetch %q", docID)
	_, err = docsClient.Documents.Get(docID).Do()
	if err == nil {
		log.Println("fetched the doc")
	} else {
		log.Println("error fetching doc: ", err)
	}

	return nil
}

func main() {
	var args struct {
		Port int `arg:"positional,env:PORT" default:"8000"` // this will not contain a leading colon due to Cloud Run API
	}
	arg.MustParse(&args)

	http.Handle("/authorizeFile", http.HandlerFunc(handleAuthorizeFile))
	http.Handle("/", http.HandlerFunc(handleRoot))

	// we must add the colon ourselves because Cloud Run will give us an integer port
	port := fmt.Sprintf(":%d", args.Port)
	log.Println("listening on " + port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Println(err)
	}
}
