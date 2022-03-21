package main

// TODO:
//   deal with tables
//   deal with equations
//   deal with first-line and hanging indents
//   do not print warning about foreground color / underlined text for links
//   do not print warning about foreground/background color when it is black/white
//   deal with block quotes

import (
	"context"
	"fmt"

	"github.com/alexflint/doc-publisher/googledoc"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type fetchGoogleDocArgs struct {
	Document string `arg:"positional"`
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

	// create the docs client
	docsClient, err := docs.NewService(ctx, option.WithTokenSource(googleToken))
	if err != nil {
		return fmt.Errorf("error creating docs client: %w", err)
	}

	// fetch the document
	d, err := googledoc.Fetch(ctx, args.Document, docsClient, driveClient)
	if err != nil {
		return err
	}

	// write to file
	err = googledoc.WriteFile(d, args.Output)
	if err != nil {
		return nil
	}

	fmt.Printf("wrote googledoc to %s\n", args.Output)
	return nil
}
