package googledoc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"

	"cloud.google.com/go/storage"
)

// regular expression for finding image references in HTML-exported google docs
var imageRegexp = regexp.MustCompile(`images\/image\d+\.(png|jpg)`)

// UploadImages uploads a set of images to cloud storage and returns a URL for each one
func UploadImages(ctx context.Context, images []*Image, bucket *storage.BucketHandle) ([]string, error) {
	// upload each image to cloud storage
	var urls []string
	for _, image := range images {
		extension := filepath.Ext(image.Filename)
		if extension == "" {
			extension = ".jpg"
		}

		// use a hash of the image content as the filename
		hash := sha256.Sum256(image.Content)
		hexhash := hex.EncodeToString(hash[:8]) // we just take the first 8 bytes for brevity
		name := hexhash + extension
		obj := bucket.Object(name)

		wr := obj.NewWriter(ctx)
		defer wr.Close()

		_, err := wr.Write(image.Content)
		if err != nil {
			return nil, fmt.Errorf("error writing %s to cloud storage: %w", image.Filename, err)
		}
		err = wr.Close()
		if err != nil {
			return nil, fmt.Errorf("error writing %s to cloud storage: %w", image.Filename, err)
		}

		// store the URL in the map
		urls = append(urls, fmt.Sprintf("https://storage.googleapis.com/%s/%s", obj.BucketName(), obj.ObjectName()))
	}
	return urls, nil
}

// MatchOBjectIDsToImages creates a map from Google Doc object IDs to the URL of the corresponding image
func MatchObjectIDsToImages(d *Archive, imageURLs []string) (map[string]string, error) {
	if len(imageURLs) != len(d.Images) {
		return nil, fmt.Errorf("google doc contained %d images but %d URLs were passed in",
			len(d.Images), len(imageURLs))
	}

	// create a map from image filenames to their URLs
	imageURLByFilename := make(map[string]string)
	for i, img := range d.Images {
		imageURLByFilename[img.Filename] = imageURLs[i]
	}

	// create a list of image filenames *in the order they appear in the HTML*
	imageFilenames := imageRegexp.FindAll(d.HTML, -1)
	var inlineObjectIDs []string
	for _, elem := range d.Doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		for _, e := range elem.Paragraph.Elements {
			if e.InlineObjectElement == nil {
				continue
			}

			obj, ok := d.Doc.InlineObjects[e.InlineObjectElement.InlineObjectId]
			if !ok {
				continue
			}

			emb := obj.InlineObjectProperties.EmbeddedObject
			if emb.EmbeddedDrawingProperties == nil && emb.ImageProperties == nil {
				continue
			}

			inlineObjectIDs = append(inlineObjectIDs, e.InlineObjectElement.InlineObjectId)
		}
	}

	// check that the number of images in the HTML matches the number of images in the document
	if len(inlineObjectIDs) != len(imageFilenames) {
		return nil, fmt.Errorf("found %d images in the HTML but %d inline objects in the document",
			len(imageFilenames), len(inlineObjectIDs))
	}

	// make a map from InlineObjectID to URL
	imageURLByObjectID := make(map[string]string)
	for i := range inlineObjectIDs {
		imageURLByObjectID[inlineObjectIDs[i]] = imageURLByFilename[string(imageFilenames[i])]
	}

	return imageURLByObjectID, nil
}
