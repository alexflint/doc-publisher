package main

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"

	"google.golang.org/api/docs/v1"
)

// googleDocImage represents an image in the HTML export of a google doc
type googleDocImage struct {
	Filename string
	Content  []byte
}

// googleDoc is the struct that is serialized to make .googledoc files
type googleDoc struct {
	Doc    *docs.Document
	HTML   []byte            // html export of the google doc
	Images []*googleDocImage // images from the html-exported google doc
}

// ReadGoogleDoc reads a .googledoc file containing a gzipped googleDoc struct
func ReadGoogleDoc(path string) (*googleDoc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening input file: %w", err)
	}
	defer f.Close()

	rd, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("error initializing gzip reader: %w", err)
	}

	var d googleDoc
	err = gob.NewDecoder(rd).Decode(&d)
	if err != nil {
		return nil, fmt.Errorf("error decoding input: %w", err)
	}
	if d.Doc == nil {
		return nil, fmt.Errorf("document was nil in decoded structure")
	}
	return &d, nil
}
