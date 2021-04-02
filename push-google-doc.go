package main

// TODO:
//   deal with emphasizing text runs that end with whitespace
//   deal with tables
//   deal with equations
//   detect code blocks

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kr/pretty"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type pushGoogleDocArgs struct {
	Document string `arg:"positional"`
}

func pushGoogleDoc(ctx context.Context, args *pushGoogleDocArgs) error {
	const tokFile = ".cache/google-push-token.json"
	googleToken, err := GoogleAuth(ctx, tokFile,
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/drive.readonly")

	// create the drive client
	driveClient, err := drive.NewService(ctx, option.WithTokenSource(googleToken))
	if err != nil {
		return fmt.Errorf("error creating drive client: %w", err)
	}

	revList, err := driveClient.Revisions.List(args.Document).Do()
	if err != nil {
		return fmt.Errorf("error getting revision list for document: %w", err)
	}
	pretty.Println(revList)
	return nil

	// create the docs client
	docsClient, err := docs.NewService(ctx, option.WithTokenSource(googleToken))
	if err != nil {
		return fmt.Errorf("error creating docs client: %w", err)
	}

	// pull the document
	existingDoc, err := docsClient.Documents.Get(args.Document).Do()
	if err != nil {
		return fmt.Errorf("error retrieving document: %w", err)
	}

	_ = existingDoc

	title := existingDoc.Body.Content[1]
	titleContent := title.Paragraph.Elements[0].TextRun.Content
	titleLength := utf8.RuneCountInString(strings.TrimSuffix(titleContent, "\n"))

	update := docs.Request{
		InsertText: &docs.InsertTextRequest{
			Text: " FOO",
			Location: &docs.Location{
				Index: title.StartIndex + int64(titleLength),
			},
		},
	}

	resp, err := docsClient.Documents.BatchUpdate(args.Document, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			&update,
		},
	}).Context(ctx).Do()
	_ = resp

	if err != nil {
		return fmt.Errorf("error updating document: %w", err)
	}

	return nil
}
