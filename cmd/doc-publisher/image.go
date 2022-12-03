package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

//go:embed secrets/storage_service_account.json
var storageServiceAccount []byte

// the google storage bucket where we store images
const imageBucket = "doc-publisher-images"

// extension should be like ".jpg", ".png"
func uploadImage(ctx context.Context, bucket *storage.BucketHandle, extension string, content []byte) (string, error) {
	// use a hash of the image content as the filename
	hash := sha256.Sum256(content)
	hexhash := hex.EncodeToString(hash[:8]) // we just take the first 8 bytes for brevity
	name := hexhash + extension
	obj := bucket.Object(name)

	// write the image to the storage bucket
	wr := obj.NewWriter(ctx)
	defer wr.Close()

	_, err := wr.Write(content)
	if err != nil {
		return "", fmt.Errorf("error writing image to cloud storage: %w", err)
	}
	err = wr.Close()
	if err != nil {
		return "", fmt.Errorf("error writing image to cloud storage: %w", err)
	}

	// construct URL for the uploaded image
	url := fmt.Sprintf("https://storage.googleapis.com/%s/%s", obj.BucketName(), obj.ObjectName())
	return url, nil
}

type pushImageArgs struct {
	Path string `arg:"positional,required"`
}

func pushImage(ctx context.Context, args *pushImageArgs) error {
	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx,
		option.WithCredentialsJSON(storageServiceAccount))
	if err != nil {
		return fmt.Errorf("error creating storage client: %w", err)
	}

	// get a handle for the image bucket
	bucket := storageClient.Bucket(imageBucket)

	// read the image
	buf, err := os.ReadFile(args.Path)
	if err != nil {
		return err
	}

	extension := filepath.Ext(args.Path)
	url, err := uploadImage(ctx, bucket, extension, buf)
	if err != nil {
		return err
	}

	fmt.Println(url)

	return nil
}
