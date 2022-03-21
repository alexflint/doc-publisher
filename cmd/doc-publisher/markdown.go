package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/alexflint/doc-publisher/googledoc"
	"github.com/alexflint/doc-publisher/markdown"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

//go:embed secrets/storage_service_account.json
var storageServiceAccount []byte

type exportMarkdownArgs struct {
	Input      string `arg:"positional"`
	SeparateBy string `help:"separate into multiple markdown files. Possible values: pagebreak"`
	Output     string `arg:"-o,--output"`
}

func exportMarkdown(ctx context.Context, args *exportMarkdownArgs) error {
	// load the document from a file
	d, err := googledoc.Load(args.Input)
	if err != nil {
		return err
	}
	fmt.Printf("loaded a googledoc with %d images\n", len(d.Images))

	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx,
		option.WithCredentialsJSON(storageServiceAccount))
	if err != nil {
		return fmt.Errorf("error creating storage client: %w", err)
	}

	imageBucket := storageClient.Bucket("doc-publisher-images")

	// upload the images
	urls, err := googledoc.UploadImages(ctx, d.Images, imageBucket)
	if err != nil {
		return err
	}

	// align the image URLs to the objects in the google doc
	imageURLsByObjectID, err := googledoc.MatchObjectIDsToImages(d, urls)
	if err != nil {
		return err
	}

	// convert and export
	switch args.SeparateBy {
	case "":
		// export the entire document as a single markdown file
		md, err := markdown.FromGoogleDoc(d.Doc, imageURLsByObjectID)
		if err != nil {
			return err
		}

		// write the result to output
		if args.Output == "" {
			fmt.Println(md)
		} else {
			err = ioutil.WriteFile(args.Output, []byte(md), 0666)
			if err != nil {
				return fmt.Errorf("error writing to %s: %w", args.Output, err)
			}
			fmt.Printf("wrote markdown to %s\n", args.Output)
		}

	case "pagebreak":
		// split the doc at page breaks into multiple markdown files
		if !strings.Contains(args.Output, "INDEX") {
			return errors.New("when using --separateby, output must be to a filename containing the string 'INDEX'")
		}

		var n int
		var cur []*docs.StructuralElement
		for i, elem := range d.Doc.Body.Content {

			// determine whether this element contains a page break
			var found bool
			if elem.Paragraph != nil {
				for _, e := range elem.Paragraph.Elements {
					if e.PageBreak != nil {
						found = true
						break
					}
				}
			}

			// if we have a page break then write out the next document
			if found || i == len(d.Doc.Body.Content)-1 {
				// convert segment of the google doc to markdown
				md, err := markdown.FromGoogleDocSegment(d.Doc, cur, imageURLsByObjectID)
				if err != nil {
					return err
				}

				// write markdown to a file
				filename := strings.ReplaceAll(args.Output, "INDEX", strconv.Itoa(n+1))
				err = ioutil.WriteFile(filename, []byte(md), 0666)
				if err != nil {
					return fmt.Errorf("error writing to %s: %w", filename, err)
				}
				fmt.Printf("wrote markdown to %s\n", filename)

				// reset the elements and increment the counter
				cur = make([]*docs.StructuralElement, 0, 1000)
				n += 1
			} else {
				cur = append(cur, elem)
			}
		}
	default:
		return fmt.Errorf("invalid value for --separateby: %q", args.SeparateBy)
	}

	return nil
}
