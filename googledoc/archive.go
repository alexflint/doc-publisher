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

// ReadFile reads a .googledoc file
func ReadFile(path string) (*Archive, error) {
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

// WriteFile writes a google doc to a .googledoc file
func WriteFile(d *Archive, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error opening output file for writing: %w", err)
	}
	defer f.Close()

	wr := gzip.NewWriter(f)
	defer wr.Close()

	err = gob.NewEncoder(wr).Encode(d)
	if err != nil {
		return fmt.Errorf("error encoding document as gob: %w", err)
	}

	err = wr.Flush()
	if err != nil {
		return fmt.Errorf("error encoding document as zip: %w", err)
	}
	return nil
}
