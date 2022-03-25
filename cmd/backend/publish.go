package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/storage"
	"github.com/alexflint/doc-publisher/googledoc"
	"github.com/alexflint/doc-publisher/lesswrong"
	"github.com/alexflint/doc-publisher/markdown"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type result struct {
	URL string
}

func publish(ctx context.Context, creds *google.Credentials, docID string, lwID string) (*result, error) {
	// create the docs client
	docsClient, err := docs.NewService(ctx,
		option.WithCredentials(creds),
		option.WithScopes(docs.DriveFileScope))
	if err != nil {
		return nil, fmt.Errorf("error creating docs client")
	}

	// create the drive client
	driveClient, err := drive.NewService(ctx,
		option.WithCredentials(creds),
		option.WithScopes(docs.DriveFileScope))
	if err != nil {
		return nil, fmt.Errorf("error creating drive client")
	}

	// fetch the google doc
	d, err := googledoc.Fetch(ctx, docID, docsClient, driveClient)
	if err != nil {
		return nil, fmt.Errorf("error fetching google doc: %w", err)
	}

	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx,
		option.WithCredentialsJSON(storageServiceAccount))
	if err != nil {
		return nil, fmt.Errorf("error creating cloud storage client")
	}

	// upload images
	imageURLs, err := googledoc.UploadImages(ctx, d.Images, storageClient.Bucket("doc-publisher-images"))
	if err != nil {
		return nil, fmt.Errorf("error uploading images: %w", err)
	}

	// match image URLs to object IDs
	imageURLsByObjectID, err := googledoc.MatchObjectIDsToImages(d, imageURLs)
	if err != nil {
		return nil, fmt.Errorf("error matching image URLs to object IDs: %w", err)
	}

	// convert to markdown
	md, err := markdown.FromGoogleDoc(d.Doc, imageURLsByObjectID)
	if err != nil {
		return nil, fmt.Errorf("error converting google doc to markdown: %w", err)
	}

	// create the lesswrong client
	lw, err := lesswrong.NewClient(ctx, "alex.flint@gmail.com", lesswrongPassword)
	if err != nil {
		return nil, fmt.Errorf("error authenticating with lesswrong: %w", err)
	}

	// create the post
	resp, err := lw.CreatePost(ctx, lesswrong.CreatePostRequest{
		Title:   "brand new post",
		Content: md,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating lesswrong post: %w", err)
	}

	log.Println("created lesswrong post: " + resp.URL)

	return &result{
		URL: resp.URL,
	}, nil
}
