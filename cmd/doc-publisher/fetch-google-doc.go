package main

// TODO:
//   deal with tables
//   deal with equations
//   deal with first-line and hanging indents
//   do not print warning about foreground color / underlined text for links
//   do not print warning about foreground/background color when it is black/white
//   deal with block quotes

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alexflint/doc-publisher/googledoc"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type fetchGoogleDocArgs struct {
	Document string `arg:"positional"`
	SaveZip  string
	Output   string `arg:"-o,--output"`
}

func fetchGoogleDoc(ctx context.Context, args *fetchGoogleDocArgs) error {
	const tokFile = ".cache/google-pull-token.json"
	googleToken, err := GoogleAuth(ctx, tokFile,
		"https://www.googleapis.com/auth/documents.readonly",
		"https://www.googleapis.com/auth/drive.readonly")
	if err != nil {
		return fmt.Errorf("error authenticating with google: %w", err)
	}

	// create the drive client
	driveClient, err := drive.NewService(ctx, option.WithTokenSource(googleToken)) // option.WithHTTPClient(googleClient))
	if err != nil {
		return fmt.Errorf("error creating drive client: %w", err)
	}

	// export the document as a zip arcive
	resp, err := driveClient.Files.Export(args.Document, "application/zip").Download()
	if err != nil {
		return fmt.Errorf("error in file download api call: %w", err)
	}

	zipbuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading exported doc from request: %w", err)
	}

	// save to disk if requested
	if args.SaveZip != "" {
		err := ioutil.WriteFile(args.SaveZip, zipbuf, 0666)
		if err != nil {
			return fmt.Errorf("error writing to %s: %w", args.SaveZip, err)
		}
		fmt.Printf("wrote %d bytes to %s\n", len(zipbuf), args.SaveZip)
	}

	// open the zip file
	ziprd, err := zip.NewReader(bytes.NewReader(zipbuf), int64(len(zipbuf)))
	if err != nil {
		return fmt.Errorf("error decoding zip archive: %w", err)
	}

	var d googledoc.Archive
	for _, f := range ziprd.File {
		if strings.HasSuffix(f.Name, ".html") {
			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			d.HTML, err = ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}
		}
		if strings.HasPrefix(f.Name, "images/image") {
			// read the image from the zip archive
			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}

			d.Images = append(d.Images, &googledoc.Image{
				Filename: f.Name,
				Content:  buf,
			})
		}
	}

	if d.HTML == nil {
		return fmt.Errorf("no html file found in downloaded zip archive")
	}

	// create the docs client
	docsClient, err := docs.NewService(ctx, option.WithTokenSource(googleToken))
	if err != nil {
		return fmt.Errorf("error creating docs client: %w", err)
	}

	// fetch the document
	d.Doc, err = docsClient.Documents.Get(args.Document).Do()
	if err != nil {
		return fmt.Errorf("error retrieving document: %w", err)
	}

	// write the document as a gzipped gob file
	f, err := os.Create(args.Output)
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

	fmt.Printf("wrote googledoc to %s\n", args.Output)
	return nil
}
