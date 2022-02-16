package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
)

const resp = `
{
	"action": {
		"navigations": [{
			"pushCard":  {
				"sections": [{
					"widgets": [
						{
							"textParagraph": {
								"text": "Hello world!"
							}
						}
					]
				}]
			}
		}]
	}
}`

func handleRoot(w http.ResponseWriter, r *http.Request) {
	http.ServeContent(w, r, "response.json", time.Time{}, strings.NewReader(resp))
}

func main() {
	var args struct {
		Port int `arg:"positional,env:PORT" default:"8000"` // this will not contain a leading colon due to Cloud Run API
	}
	arg.MustParse(&args)

	http.Handle("/", http.HandlerFunc(handleRoot))

	// we must add the colon ourselves because Cloud Run will give us an integer port
	port := fmt.Sprintf(":%d", args.Port)
	log.Println("listening on " + port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Println(err)
	}
}
