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

// determine whether a font is monospace (for detecting code blocks)
func isMonospace(font *docs.WeightedFontFamily) bool {
	if font == nil {
		return false
	}

	switch strings.ToLower(font.FontFamily) {
	case "courier new":
		return true
	case "consolas":
		return true
	case "roboto mono":
		return true
	}
	return false
}

type pullGoogleDocArgs struct {
	Document string
	SaveZip  string
	Output   string `arg:"-o,--output"`
}

func pullGoogleDoc(ctx context.Context, args *pullGoogleDocArgs) error {
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
			// read the image from the zip archive
			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening %s from zip archive: %w", f.Name, err)
			}

			buf, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("error reading %s from zip archive: %w", f.Name, err)
			}

			// write the image to cloud storage
			hash := sha256.Sum256(buf)
			hexhash := hex.EncodeToString(hash[:8]) // we just take the first 8 bytes for brevity
			name := hexhash + ".jpg"
			obj := imageBucket.Object(name)

			wr := obj.NewWriter(ctx)
			defer wr.Close()

			_, err = wr.Write(buf)
			if err != nil {
				return fmt.Errorf("error writing %s to cloud storage: %w", f.Name, err)
			}
			wr.Close()

			// store the URL in the map
			imgURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", obj.BucketName(), obj.ObjectName())
			imageURLByFilename[f.Name] = imgURL
			fmt.Printf("%s => %s\n", f.Name, imgURL)
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

	// convert the document to markdown
	converter := docConverter{
		doc: doc,
	}
	for _, imageFilename := range imageOrder {
		converter.imageURLs = append(converter.imageURLs, imageURLByFilename[imageFilename])
	}

	// process the main body content
	var markdown bytes.Buffer
	err = converter.process(&markdown, doc.Body.Content)
	if err != nil {
		return fmt.Errorf("error converting document body to markdown: %w", err)
	}

	// process each footnote
	for _, footnote := range doc.Footnotes {
		var footnoteMarkdown bytes.Buffer
		err = converter.process(&footnoteMarkdown, footnote.Content)
		if err != nil {
			return fmt.Errorf("error converting footnote %s content to markdown: %w", footnote.FootnoteId, err)
		}

		fmt.Fprintf(&markdown, "[^%s]: ", footnote.FootnoteId)
		for i, line := range strings.Split(footnoteMarkdown.String(), "\n") {
			if i > 0 {
				fmt.Fprint(&markdown, "    ") // multi-line footnotes in markdown must be indented
			}

			fmt.Fprintln(&markdown, line)
		}

		fmt.Fprint(&markdown, "\n") // make sure there is an empty line between each footnote
	}

	// drop sequences of more than 3 newlines
	var consecutiveNewlines int
	var final strings.Builder
	final.Grow(markdown.Len())
	for {
		r, _, err := markdown.ReadRune()
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

type docConverter struct {
	doc        *docs.Document
	imageURLs  []string
	imageIndex int
	codeBlock  bytes.Buffer // text identified as lines of code
}

func (dc *docConverter) process(out *bytes.Buffer, content []*docs.StructuralElement) error {
	// walk the document
	for _, elem := range content {
		switch {
		case elem.Table != nil:
			// TODO: implement
			dc.flushCodeBlock(out)
			log.Println("warning: ignoring table")
		case elem.TableOfContents != nil:
			// TODO: implement
			dc.flushCodeBlock(out)
			log.Println("warning: ignoring table of contents")
		case elem.SectionBreak != nil:
			dc.flushCodeBlock(out)
			log.Println("warning: ignoring section break")
		case elem.Paragraph != nil:
			err := dc.processParagraph(out, elem.Paragraph)
			if err != nil {
				return err
			}
		default:
			log.Println("warning: encountered a body element of unknown type")
		}
	}

	// flush any remaining code block
	dc.flushCodeBlock(out)

	return nil
}

// flushCodeBlock writes any lines stored in dc.codeblock to a markdown
// code block, or if there are no stored lines then it does nothing
func (dc *docConverter) flushCodeBlock(out *bytes.Buffer) {
	if dc.codeBlock.Len() == 0 {
		return
	}

	fmt.Fprintln(out, "```")
	dc.codeBlock.WriteTo(out)
	fmt.Fprintln(out, "```")
	fmt.Fprintln(out)
	dc.codeBlock.Reset()
}

func (dc *docConverter) processParagraph(out *bytes.Buffer, p *docs.Paragraph) error {
	// deal with code blocks
	isCode := p.ParagraphStyle.NamedStyleType == "NORMAL_TEXT" && p.Bullet == nil
	if isCode {
		for _, el := range p.Elements {
			if el.TextRun == nil {
				isCode = false
				break
			}
			if !isMonospace(el.TextRun.TextStyle.WeightedFontFamily) {
				isCode = false
				break
			}
		}
	}

	if isCode {
		for _, el := range p.Elements {
			fmt.Fprint(&dc.codeBlock, el.TextRun.Content)
		}
		return nil
	}

	// if not a code block then flush any buffered code block
	dc.flushCodeBlock(out)

	// print the heading prefix
	var isHeading bool
	switch p.ParagraphStyle.NamedStyleType {
	case "TITLE":
		isHeading = true
		fmt.Fprintf(out, "# ")
	case "HEADING_1":
		isHeading = true
		fmt.Fprintf(out, "# ")
	case "HEADING_2":
		isHeading = true
		fmt.Fprintf(out, "## ")
	case "HEADING_3":
		isHeading = true
		fmt.Fprintf(out, "### ")
	case "HEADING_4":
		isHeading = true
		fmt.Fprintf(out, "#### ")
	case "HEADING_5":
		isHeading = true
		fmt.Fprintf(out, "##### ")
	case "HEADING_6":
		isHeading = true
		fmt.Fprintf(out, "###### ")
	}

	// deal with bullets
	if p.Bullet != nil {
		if isHeading {
			fmt.Println("found a heading that is part of a bulletted list, ignoring the bullet")
		} else {
			list := dc.doc.Lists[p.Bullet.ListId]
			level := list.ListProperties.NestingLevels[p.Bullet.NestingLevel]

			var i int64
			for i = 0; i < p.Bullet.NestingLevel; i++ {
				fmt.Fprintf(out, "  ")
			}

			// if there is no fixed glyph symbol then this is an ordered list
			if level.GlyphSymbol == "" {
				fmt.Fprintf(out, "1. ")
			} else {
				fmt.Fprintf(out, "* ")
			}
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
			fmt.Fprintf(out, "[^%s]", el.FootnoteReference.FootnoteId)
		case el.AutoText != nil:
			log.Println("warning: ignoring auto text")
		case el.HorizontalRule != nil:
			fmt.Fprintf(out, "\n---\n")
		case el.InlineObjectElement != nil:
			err := dc.processInlineObject(out, el.InlineObjectElement)
			if err != nil {
				return err
			}
		case el.PageBreak != nil:
			log.Println("  page break")
		case el.TextRun != nil:
			err := dc.processTextRun(out, el.TextRun)
			if err != nil {
				return err
			}
		default:
			log.Println("warning: encountered a paragraph element of unknown type")
		}
	}

	// write two newlines at the end of each paragraph
	fmt.Fprint(out, "\n\n")
	return nil
}

func (dc *docConverter) processInlineObject(out *bytes.Buffer, objRef *docs.InlineObjectElement) error {
	id := objRef.InlineObjectId
	obj, ok := dc.doc.InlineObjects[id]
	if !ok {
		fmt.Println("warning: could not find inline object for id", id)
		return nil
	}

	emb := obj.InlineObjectProperties.EmbeddedObject
	switch {
	case emb.ImageProperties != nil || emb.EmbeddedDrawingProperties != nil:
		if dc.imageIndex >= len(dc.imageURLs) {
			return fmt.Errorf("found %d images in zip but too many objects in the doc", len(dc.imageURLs))
		}
		fmt.Fprintf(out, "![%s](%s)", emb.Title, dc.imageURLs[dc.imageIndex])
		dc.imageIndex++
	case emb.LinkedContentReference != nil:
		log.Println("warning: ignoring linked spreadsheet / chart")
	}

	return nil
}

func (dc *docConverter) processTextRun(out *bytes.Buffer, t *docs.TextRun) error {
	// unfortunately markdown only supports at most one of italic, bold,
	// or strikethrough for any one bit of text
	var surround string
	if t.TextStyle.Italic {
		surround = "*"
	}
	if t.TextStyle.Bold {
		surround = "**"
	}
	if t.TextStyle.Strikethrough {
		surround = "-"
	}
	if isMonospace(t.TextStyle.WeightedFontFamily) {
		surround = "`"
	}

	// the following features are not supported at all in markdown
	if t.TextStyle.Underline {
		log.Printf("warning: ignoring underlined text (%q)", t.Content)
	}
	if t.TextStyle.SmallCaps {
		log.Printf("warning: ignoring smallcaps (%q)", t.Content)
	}
	if t.TextStyle.BackgroundColor != nil {
		log.Printf("warning: ignoring text with background color (%q)", t.Content)
	}
	if t.TextStyle.ForegroundColor != nil {
		log.Printf("warning: ignoring text with foreground color (%q)", t.Content)
	}
	switch t.TextStyle.BaselineOffset {
	case "SUBSCRIPT":
		log.Println("warning: ignoring subscript")
	case "SUPERSCRIPT":
		log.Println("warning: ignoring superscript")
	}

	// replace unicode quote characters with ordinary quote characters
	content := t.Content
	content = strings.Replace(content, `“`, `"`, -1)
	content = strings.Replace(content, `”`, `"`, -1)

	// in markdown we must apply styling separately to each line
	link := t.TextStyle.Link
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}

		if link != nil {
			fmt.Fprint(out, "[")
		}

		fmt.Fprintf(out, surround)
		fmt.Fprintf(out, line)
		fmt.Fprintf(out, surround)

		if link != nil {
			fmt.Fprintf(out, "](%s)", link.Url)
		}

		if i+1 < len(lines) {
			fmt.Fprintf(out, "\n")
		}
	}

	return nil
}
