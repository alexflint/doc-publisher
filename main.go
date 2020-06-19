package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/alexflint/doc-publisher/imgur"
	"github.com/alexflint/doc-publisher/lesswrong"
	"github.com/alexflint/go-arg"
	"github.com/kr/pretty"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v2"
	"google.golang.org/api/option"
)

func fail(msg interface{}, parts ...interface{}) {
	fmt.Printf(fmt.Sprintf("%v\n", msg), parts...)
	os.Exit(1)
}

// Requests a token from the web, then returns the retrieved token.
func tokenFromOauth(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token from web: %v", err)
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

const imgurAPIKey = "ac9d94aa284ff2a"

func main() {
	ctx := context.Background()

	// parse command line args
	var args struct {
		Password string `arg:"-p,--password"`
		Document string
		Upload   string
		SaveZip  string
	}
	//args.Document = "1_4OtBmq2gG8zFnqTlAvpHc1sshfkv4hw3z62vHs4crI" // scratch document
	args.Document = "1px3ivo6aFqAi0TA4u9oJkxwsry1D5GYv76GZ4nV00Rk" // ground of optimization
	arg.MustParse(&args)

	imgur := imgur.New(imgurAPIKey)

	// upload an image to imgur
	if args.Upload != "" {
		buf, err := ioutil.ReadFile(args.Upload)
		if err != nil {
			fail(err)
		}

		imageURL, err := imgur.Upload(ctx, buf)
		if err != nil {
			fail(err)
		}

		fmt.Println(imageURL)

		return
	}

	if args.Password != "" {
		// authenticate to lesswrong
		auth, err := lesswrong.Authenticate(ctx, "alex.flint@gmail.com", args.Password)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("got auth token:", auth.Token)

		lw, err := lesswrong.New()
		lw.Auth = auth

		// err = lw.CreatePost(ctx)
		// if err != nil {
		// 	log.Fatal(err)
		// }

		// fmt.Println("done")
		// os.Exit(0)
	}

	// authenticate to google
	b, err := ioutil.ReadFile("oauth_credentials.json")
	if err != nil {
		fail("unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b,
		"https://www.googleapis.com/auth/documents.readonly",
		"https://www.googleapis.com/auth/drive.readonly")
	if err != nil {
		fail("unable to parse client secret file to config: %v", err)
	}

	// do the oauth flow
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = tokenFromOauth(ctx, config)
		if err != nil {
			fail("error in oauth flow: %v", err)
		}
		err = saveToken(tokFile, tok)
		if err != nil {
			fail("error saving token to file: %v", err)
		}
	}
	client := config.Client(context.Background(), tok)

	// create the drive client
	driveClient, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		fail("error creating drive client: %v", err)
	}

	// export the document as a zip arcive
	resp, err := driveClient.Files.Export(args.Document, "application/zip").Download()
	if err != nil {
		fail("error in file download api call: %v", err)
	}

	zipbuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fail("error reading exported doc from request: %v", err)
	}

	// save to disk if requested
	if args.SaveZip != "" {
		err := ioutil.WriteFile(args.SaveZip, zipbuf, os.ModePerm)
		if err != nil {
			fail("error writing to %s: %w", args.SaveZip, err)
		}
		fmt.Printf("wrote %d bytes to %s\n", len(zipbuf), args.SaveZip)
	}

	// upload each image to imgur
	ziprd, err := zip.NewReader(bytes.NewReader(zipbuf), int64(len(zipbuf)))
	if err != nil {
		fail("error decoding zip archive: %v", err)
	}

	imgURLs := make(map[int]string)
	for _, f := range ziprd.File {
		if strings.HasPrefix(f.Name, "images/image") {
			fmt.Println(f.Name)

			s := strings.TrimPrefix(f.Name, "images/image")
			s = strings.TrimSuffix(s, ".png")
			n, err := strconv.Atoi(s)
			if err != nil {
				fail("error interpreting image filename %q: expected 'images/imageN.png'", f.Name)
			}

			r, err := f.Open()
			if err != nil {
				fail("error opening %s from zip archive: %v", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				fail("error reading %s from zip archive: %v", f.Name, err)
			}

			imgURL, err := imgur.Upload(ctx, buf)
			if err != nil {
				fail("error uploading %s to imgur: %v", f.Name, err)
			}

			imgURLs[n] = imgURL
		}
	}

	pretty.Println(imgURLs)

	return

	// create the docs client
	docsClient, err := docs.New(client)
	if err != nil {
		fail("error creating docs client: %v", err)
	}

	// fetch the document
	doc, err := docsClient.Documents.Get(args.Document).Do()
	if err != nil {
		fail("error retrieving document: %v", err)
	}
	fmt.Println(doc.Title)

	// walk the document
	for _, elem := range doc.Body.Content {
		switch {
		case elem.Paragraph != nil:
			fmt.Println("paragraph")
			p := elem.Paragraph
			for _, el := range p.Elements {
				switch {
				case el.ColumnBreak != nil:
					fmt.Println("  column break")
				case el.Equation != nil:
					fmt.Println("  equation")
				case el.FootnoteReference != nil:
					fmt.Println("  footnote ref")
				case el.HorizontalRule != nil:
					fmt.Println("  horizontal rule")
				case el.InlineObjectElement != nil:
					localID := el.InlineObjectElement.InlineObjectId
					obj, ok := doc.InlineObjects[localID]
					if !ok {
						fmt.Println("could not find inline object for id", localID)
						continue
					}
					emb := obj.InlineObjectProperties.EmbeddedObject
					if emb == nil {
						fmt.Println("not an embedded objet")
						continue
					}
					if emb.EmbeddedDrawingProperties == nil {
						fmt.Println("not a drawing")
						continue
					}
					fmt.Println("found a drawing:")
					pretty.Println(emb)

				case el.PageBreak != nil:
					fmt.Println("  page break")
				case el.TextRun != nil:
					fmt.Println("  text run")
				}
			}
		case elem.Table != nil:
			fmt.Println("table")
		case elem.TableOfContents != nil:
			fmt.Println("toc")
		case elem.SectionBreak != nil:
			fmt.Println("section break")
		default:
			fmt.Println("unknown!")
		}
	}
}
