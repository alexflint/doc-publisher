package googledoc

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"

	"google.golang.org/api/docs/v1"
)

// Archive represents a google that has been exported, including images
// This is the struct that is serialized to make .googledoc files
type Archive struct {
	Doc    *docs.Document
	HTML   []byte   // html export of the google doc
	Images []*Image // images from the html-exported google doc
}

// Image represents an image in the HTML export of a google doc
type Image struct {
	Filename string
	Content  []byte
}

// ReadGoogleDoc reads a .googledoc file containing a gzipped googleDoc struct
func Load(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening input file: %w", err)
	}
	defer f.Close()

	rd, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("error initializing gzip reader: %w", err)
	}

	var d Archive
	err = gob.NewDecoder(rd).Decode(&d)
	if err != nil {
		return nil, fmt.Errorf("error decoding input: %w", err)
	}
	if d.Doc == nil {
		return nil, fmt.Errorf("document was nil in decoded structure")
	}
	return &d, nil
}
