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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/alexflint/doc-publisher/imgur"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v2"
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

// Requests a token from the web, then returns the retrieved token.
func tokenFromOauth(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token from web: %w", err)
	}
	return tok, nil
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer f.Close()
	if err != nil {
		return nil, err
	}

	var tok oauth2.Token
	err = json.NewDecoder(f).Decode(&tok)
	if err != nil {
		return nil, err
	}

	return &tok, nil
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer f.Close()
	if err != nil {
		return err
	}

	return json.NewEncoder(f).Encode(token)
}

type pullGoogleDocArgs struct {
	Document    string
	SaveZip     string
	Output      string `arg:"-o,--output"`
	ImgurAPIKey string `arg:"--imgur-api-key,env:IMGUR_API_KEY"`
}

func pullGoogleDoc(ctx context.Context, args *pullGoogleDocArgs) error {
	if args.ImgurAPIKey == "" {
		return errors.New("please specify an imgur API key with --imgur-api-key")
	}

	imgur := imgur.New(args.ImgurAPIKey)

	// authenticate to google
	b, err := ioutil.ReadFile("oauth_credentials.json")
	if err != nil {
		return fmt.Errorf("unable to read client secret file: %w", err)
	}

	config, err := google.ConfigFromJSON(b,
		"https://www.googleapis.com/auth/documents.readonly",
		"https://www.googleapis.com/auth/drive.readonly")
	if err != nil {
		return fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	// do the oauth flow
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = tokenFromOauth(ctx, config)
		if err != nil {
			return fmt.Errorf("error in oauth flow: %w", err)
		}
		err = saveToken(tokFile, tok)
		if err != nil {
			return fmt.Errorf("error saving token to file: %w", err)
		}
	}
	client := config.Client(context.Background(), tok)

	// create the drive client
	driveClient, err := drive.NewService(ctx, option.WithHTTPClient(client))
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

	// upload each image to imgur
	ziprd, err := zip.NewReader(bytes.NewReader(zipbuf), int64(len(zipbuf)))
	if err != nil {
		return fmt.Errorf("error decoding zip archive: %w", err)
	}

	// open the image URL cache
	err = os.MkdirAll(".image-url-cache", os.ModePerm)
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
			hexhash := hex.EncodeToString(hash[:])
			cachepath := ".image-url-cache/" + hexhash
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
				imgURL, err := imgur.Upload(ctx, buf)
				if err != nil {
					return fmt.Errorf("error uploading %s to imgur: %w", f.Name, err)
				}

				err = ioutil.WriteFile(cachepath, []byte(imgURL), 0666)
				if err != nil {
					return fmt.Errorf("error storing uploaded image path to cache: %w", err)
				}
			}

			imageURLByFilename[f.Name] = imgURL
		}
	}

	if !foundHTML {
		return fmt.Errorf("no html file found in downloaded zip archive")
	}

	fmt.Println(imageOrder)

	// create the docs client
	docsClient, err := docs.New(client)
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
