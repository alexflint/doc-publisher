package googledoc

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// Fetch fetches a google doc using Google's REST API
func Fetch(ctx context.Context, docID string, docsClient *docs.Service, driveClient *drive.Service) (*Archive, error) {
	// export the document as a zip arcive
	resp, err := driveClient.Files.Export(docID, "application/zip").Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("error in file download api call: %w", err)
	}

	zipbuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading exported doc from request: %w", err)
	}

	// open the zip file
	ziprd, err := zip.NewReader(bytes.NewReader(zipbuf), int64(len(zipbuf)))
	if err != nil {
		return nil, fmt.Errorf("error decoding zip archive: %w", err)
	}

	var d Archive
	for _, f := range ziprd.File {
		if strings.HasSuffix(f.Name, ".html") {
			r, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			d.HTML, err = ioutil.ReadAll(r)
			if err != nil {
				return nil, fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}
		}
		if strings.HasPrefix(f.Name, "images/image") {
			// read the image from the zip archive
			r, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				return nil, fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}

			d.Images = append(d.Images, &Image{
				Filename: f.Name,
				Content:  buf,
			})
		}
	}

	if d.HTML == nil {
		return nil, fmt.Errorf("no html file found in downloaded zip archive")
	}

	// fetch the document
	d.Doc, err = docsClient.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("error retrieving document: %w", err)
	}

	return &d, nil
}
