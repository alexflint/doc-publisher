package main

// TODO:
//   deal with emphasizing text runs that end with whitespace
//   deal with tables
//   deal with equations
//   detect code blocks

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// reverse reverses a string
func reverse(s string) string {
	rs := make([]rune, len(s))
	var n int
	for i, r := range s {
		rs[len(s)-i-1] = r
		n++
	}
	return string(rs[len(s)-n : len(s)])
}

type pullGoogleDocArgs struct {
	Document string
	SaveZip  string
	Output   string `arg:"-o,--output"`
	//ImgurAPIKey string `arg:"--imgur-api-key,env:IMGUR_API_KEY"`
}

func pullGoogleDoc(ctx context.Context, args *pullGoogleDocArgs) error {
	// if args.ImgurAPIKey == "" {
	// 	return errors.New("please specify an imgur API key with --imgur-api-key")
	// }

	// imgur := imgur.New(args.ImgurAPIKey)

	const tokFile = ".cache/google-pull-token.json"
	googleToken, err := GoogleAuth(ctx, tokFile,
		"https://www.googleapis.com/auth/documents.readonly",
		"https://www.googleapis.com/auth/drive.readonly")

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

	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error creating storage client: %w", err)
	}

	imageBucket := storageClient.Bucket("doc-publisher-images")

	// open the zip file
	ziprd, err := zip.NewReader(bytes.NewReader(zipbuf), int64(len(zipbuf)))
	if err != nil {
		return fmt.Errorf("error decoding zip archive: %w", err)
	}

	// open the image URL cache
	err = os.MkdirAll(".cache/image-urls", os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating image URL cache dir: %w", err)
	}

	var foundHTML bool
	var imageOrder []string
	imageURLByFilename := make(map[string]string)
	for _, f := range ziprd.File {
		if strings.HasSuffix(f.Name, ".html") {
			foundHTML = true
			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}

			re, err := regexp.Compile(`images\/image\d+\.png`)
			if err != nil {
				return fmt.Errorf("error compiling regexp: %w", err)
			}

			matches := re.FindAll(buf, -1)
			for _, m := range matches {
				imageOrder = append(imageOrder, string(m))
			}
		}
		if strings.HasPrefix(f.Name, "images/image") {
			fmt.Println(f.Name)

			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}

			hash := sha256.Sum256(buf)
			hexhash := hex.EncodeToString(hash[:8]) // we just take the first 8 bytes for brevity
			cachepath := ".cache/image-urls/" + hexhash
			urlbuf, err := ioutil.ReadFile(cachepath)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("error reading from %s: %w", cachepath, err)
			}

			var imgURL string
			if len(urlbuf) > 0 {
				// cache hit
				imgURL = string(urlbuf)
			} else {
				// cache miss
				name := hexhash + ".jpg"
				obj := imageBucket.Object(name)
				wr := obj.NewWriter(ctx)
				defer wr.Close()

				_, err := wr.Write(buf)
				if err != nil {
					return fmt.Errorf("error writing %s to cloud storage: %w", f.Name, err)
				}
				wr.Close()

				imgURL = fmt.Sprintf("https://storage.googleapis.com/%s/%s", obj.BucketName(), obj.ObjectName())

				err = ioutil.WriteFile(cachepath, []byte(imgURL), 0666)
				if err != nil {
					return fmt.Errorf("error storing uploaded image path to cache: %w", err)
				}
			}

			fmt.Printf("%s => %s\n", f.Name, imgURL)

			imageURLByFilename[f.Name] = imgURL
		}
	}

	if !foundHTML {
		return fmt.Errorf("no html file found in downloaded zip archive")
	}

	fmt.Println(imageOrder)

	// create the docs client
	docsClient, err := docs.NewService(ctx, option.WithTokenSource(googleToken))
	if err != nil {
		return fmt.Errorf("error creating docs client: %w", err)
	}

	// fetch the document
	doc, err := docsClient.Documents.Get(args.Document).Do()
	if err != nil {
		return fmt.Errorf("error retrieving document: %w", err)
	}

	// walk the document
	var imageIndex int
	var md bytes.Buffer
	for _, elem := range doc.Body.Content {
		switch {
		case elem.Table != nil:
			// TODO: implement
			log.Println("warning: ignoring table")
		case elem.TableOfContents != nil:
			// TODO: implement
			log.Println("warning: ignoring table of contents")
		case elem.SectionBreak != nil:
			log.Println("warning: ignoring section break")
		case elem.Paragraph != nil:
			p := elem.Paragraph

			// print the heading
			switch p.ParagraphStyle.NamedStyleType {
			case "TITLE":
				fmt.Fprintf(&md, "# ")
			case "HEADING_1":
				fmt.Fprintf(&md, "# ")
			case "HEADING_2":
				fmt.Fprintf(&md, "## ")
			case "HEADING_3":
				fmt.Fprintf(&md, "### ")
			case "HEADING_4":
				fmt.Fprintf(&md, "#### ")
			case "HEADING_5":
				fmt.Fprintf(&md, "##### ")
			case "HEADING_6":
				fmt.Fprintf(&md, "###### ")
			}

			if p.Bullet != nil {
				list := doc.Lists[p.Bullet.ListId]
				level := list.ListProperties.NestingLevels[p.Bullet.NestingLevel]

				var i int64
				for i = 0; i < p.Bullet.NestingLevel; i++ {
					fmt.Fprintf(&md, "  ")
				}

				// if there is no fixed glyph symbol then this is an ordered list
				if level.GlyphSymbol == "" {
					fmt.Fprintf(&md, "1. ")
				} else {
					fmt.Fprintf(&md, "* ")
				}
			}

			// print each text run in the paragraph
			for _, el := range p.Elements {
				switch {
				case el.ColumnBreak != nil:
					log.Println("warning: ignoring column break")
				case el.Equation != nil:
					// TODO: implement
					log.Println("warning: ignoring equation")
				case el.FootnoteReference != nil:
					// TODO: implement
					log.Println("warning: ignoring footnote")
				case el.AutoText != nil:
					log.Println("warning: ignoring auto text")
				case el.HorizontalRule != nil:
					fmt.Fprintf(&md, "\n---\n")
				case el.InlineObjectElement != nil:
					id := el.InlineObjectElement.InlineObjectId
					obj, ok := doc.InlineObjects[id]
					if !ok {
						fmt.Println("warning: could not find inline object for id", id)
						continue
					}

					emb := obj.InlineObjectProperties.EmbeddedObject
					switch {
					case emb.ImageProperties != nil || emb.EmbeddedDrawingProperties != nil:
						if imageIndex >= len(imageOrder) {
							return fmt.Errorf("found %d images in zip but too many objects in the doc", len(imageOrder))
						}
						imageFilename := imageOrder[imageIndex]
						imageURL, ok := imageURLByFilename[imageFilename]
						if !ok {
							return fmt.Errorf("no URL found for %s", imageFilename)
						}
						fmt.Fprintf(&md, "![%s](%s)", emb.Title, imageURL)
						imageIndex++
					case emb.LinkedContentReference != nil:
						log.Println("warning: ignoring linked spreadsheet / chart")
					}

				case el.PageBreak != nil:
					log.Println("  page break")
				case el.TextRun != nil:
					var surround string
					if el.TextRun.TextStyle.Italic {
						surround = "*"
					}
					if el.TextRun.TextStyle.Bold {
						surround = "**"
					}
					if el.TextRun.TextStyle.Strikethrough {
						surround = "-"
					}
					if el.TextRun.TextStyle.Underline {
						log.Printf("warning: ignoring underlined text (%q)", el.TextRun.Content)
					}
					if el.TextRun.TextStyle.SmallCaps {
						log.Printf("warning: ignoring smallcaps (%q)", el.TextRun.Content)
					}
					if el.TextRun.TextStyle.BackgroundColor != nil {
						log.Printf("warning: ignoring text with background color (%q)", el.TextRun.Content)
					}
					if el.TextRun.TextStyle.ForegroundColor != nil {
						log.Printf("warning: ignoring text with foreground color (%q)", el.TextRun.Content)
					}

					switch el.TextRun.TextStyle.BaselineOffset {
					case "SUBSCRIPT":
						log.Println("warning: ignoring subscript")
					case "SUPERSCRIPT":
						log.Println("warning: ignoring superscript")
					}

					content := el.TextRun.Content
					content = strings.Replace(content, `“`, `"`, -1)
					content = strings.Replace(content, `”`, `"`, -1)

					link := el.TextRun.TextStyle.Link

					// in markdown we must apply styling separately to each line
					lines := strings.Split(content, "\n")
					for i, line := range lines {
						if len(line) == 0 {
							continue
						}

						if link != nil {
							fmt.Fprint(&md, "[")
						}

						fmt.Fprintf(&md, surround)
						fmt.Fprintf(&md, line)
						fmt.Fprintf(&md, reverse(surround))

						if link != nil {
							fmt.Fprintf(&md, "](%s)", link.Url)
						}

						if i+1 < len(lines) {
							fmt.Fprintf(&md, "\n")
						}
					}

				default:
					log.Println("warning: encountered a paragraph element of unknown type")
				}
			}
			fmt.Fprint(&md, "\n\n")
		default:
			log.Println("warning: encountered a body element of unknown type")
		}
	}

	// drop sequences of more than 3 newlines
	var consecutiveNewlines int
	var final strings.Builder
	final.Grow(md.Len())
	for {
		r, _, err := md.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading runes from markdown buffer: %w", err)
		}

		if r == '\n' {
			consecutiveNewlines++
		} else {
			consecutiveNewlines = 0
		}

		if consecutiveNewlines <= 2 {
			final.WriteRune(r)
		}
	}

	if args.Output == "" {
		fmt.Println(final.String())
	} else {
		err = ioutil.WriteFile(args.Output, []byte(final.String()), 0666)
		if err != nil {
			return fmt.Errorf("error writing markdown to %s: %w", args.Output, err)
		}
	}

	return nil
}
