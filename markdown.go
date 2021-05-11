package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"cloud.google.com/go/storage"
	"google.golang.org/api/docs/v1"
)

type exportMarkdownArgs struct {
	Input      string `arg:"positional"`
	SeparateBy string `help:"separate into multiple markdown files. Possible values: pagebreak"`
	Output     string `arg:"-o,--output"`
}

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

// splitSpace splits a string into leading whitespace, trailing
// whitespace, and everything inbetween
func splitSpace(s string) (left, middle, right string) {
	for _, r := range s {
		if unicode.IsSpace(r) {
			right += string(r)
		} else if len(middle) == 0 {
			left = right
			right = ""
			middle += string(r)
		} else {
			middle += right + string(r)
			right = ""
		}
	}
	return
}

// regular expression for finding image references in HTML-exported google docs
var imageRegexp = regexp.MustCompile(`images\/image\d+\.png`)

func exportMarkdown(ctx context.Context, args *exportMarkdownArgs) error {
	// load the document from a file
	d, err := ReadGoogleDoc(args.Input)
	if err != nil {
		return err
	}

	// look at the order in which the images appear in the HTML
	var imageOrder []string
	matches := imageRegexp.FindAll(d.HTML, -1)
	for _, m := range matches {
		imageOrder = append(imageOrder, string(m))
	}

	// create a cloud storage client
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error creating storage client: %w", err)
	}

	imageBucket := storageClient.Bucket("doc-publisher-images")
	imageURLByFilename := make(map[string]string)

	// upload each image to cloud storage
	for _, image := range d.Images {
		// use a hash of the image content as the filename
		hash := sha256.Sum256(image.Content)
		hexhash := hex.EncodeToString(hash[:8]) // we just take the first 8 bytes for brevity
		name := hexhash + ".jpg"
		obj := imageBucket.Object(name)

		wr := obj.NewWriter(ctx)
		defer wr.Close()

		_, err = wr.Write(image.Content)
		if err != nil {
			return fmt.Errorf("error writing %s to cloud storage: %w", image.Filename, err)
		}
		err = wr.Close()
		if err != nil {
			return fmt.Errorf("error writing %s to cloud storage: %w", image.Filename, err)
		}

		// store the URL in the map
		imgURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", obj.BucketName(), obj.ObjectName())
		imageURLByFilename[image.Filename] = imgURL
		fmt.Printf("%s => %s\n", image.Filename, imgURL)
	}

	type job struct {
		elements []*docs.StructuralElement
		filename string // filename or empty for stdout
	}

	var jobs []job

	switch args.SeparateBy {
	case "":
		// by default we process the whole document into a single markdown file
		jobs = append(jobs, job{
			elements: d.Doc.Body.Content,
			filename: args.Output,
		})
	case "pagebreak":
		if !strings.Contains(args.Output, "INDEX") {
			return errors.New("when using --separateby, output must be to a filename containing the string 'INDEX'")
		}

		var cur []*docs.StructuralElement
		for i, elem := range d.Doc.Body.Content {
			var found bool
			if elem.Paragraph != nil {
				for _, e := range elem.Paragraph.Elements {
					if e.PageBreak != nil {
						found = true
						break
					}
				}
			}
			if found || i == len(d.Doc.Body.Content)-1 {
				jobs = append(jobs, job{
					elements: cur,
					filename: strings.ReplaceAll(args.Output, "INDEX", strconv.Itoa(len(jobs)+1)),
				})
				cur = make([]*docs.StructuralElement, 0, 1000)
			} else {
				cur = append(cur, elem)
			}
		}
	default:
		return fmt.Errorf("invalid value for --separateby: %q", args.SeparateBy)
	}

	for _, job := range jobs {
		// convert the document to markdown
		conv := markdownConverter{
			doc: d.Doc,
		}
		for _, imageFilename := range imageOrder {
			conv.imageURLs = append(conv.imageURLs, imageURLByFilename[imageFilename])
		}

		// process the main body content
		var markdown bytes.Buffer
		err = conv.process(&markdown, job.elements)
		if err != nil {
			return fmt.Errorf("error converting document body to markdown: %w", err)
		}

		// process each footnote
		for _, footnoteID := range conv.footnotes {
			footnote, ok := d.Doc.Footnotes[footnoteID]
			if !ok {
				fmt.Printf("warning: no content found for footnote %q referenced in document", footnoteID)
				continue
			}

			var footnoteMarkdown bytes.Buffer
			err = conv.process(&footnoteMarkdown, footnote.Content)
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

		// drop sequences of three or more consecutive newlines
		var newlines int
		var whitespace string
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
				if newlines < 2 {
					final.WriteRune(r)
				}
				newlines++
				whitespace = "" // drop trailing whitespace
			} else if unicode.IsSpace(r) {
				whitespace += string(r)
			} else {
				final.WriteString(whitespace)
				final.WriteRune(r)
				newlines = 0
				whitespace = ""
			}
		}

		if job.filename == "" {
			fmt.Println(final.String())
		} else {
			err = ioutil.WriteFile(job.filename, []byte(final.String()), 0666)
			if err != nil {
				return fmt.Errorf("error writing to %s: %w", job.filename, err)
			}
			fmt.Printf("wrote markdown to %s\n", job.filename)
		}
	}

	return nil
}

type markdownConverter struct {
	doc        *docs.Document
	imageURLs  []string
	imageIndex int
	codeBlock  bytes.Buffer // text identified as lines of code
	footnotes  []string     // footnote IDs processed by this converter
}

func (dc *markdownConverter) process(out *bytes.Buffer, content []*docs.StructuralElement) error {
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
func (dc *markdownConverter) flushCodeBlock(out *bytes.Buffer) {
	if dc.codeBlock.Len() == 0 {
		return
	}

	fmt.Fprintln(out, "```")
	dc.codeBlock.WriteTo(out)
	fmt.Fprintln(out, "```")
	fmt.Fprintln(out)
	dc.codeBlock.Reset()
}

func (dc *markdownConverter) processParagraph(out *bytes.Buffer, p *docs.Paragraph) error {
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
			// add this footnote ID if it is not already in the list
			var found bool
			for _, f := range dc.footnotes {
				if f == el.FootnoteReference.FootnoteId {
					found = true
					break
				}
			}
			if !found {
				dc.footnotes = append(dc.footnotes, el.FootnoteReference.FootnoteId)
			}
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
			log.Println("warning: ignoring page break")
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

func (dc *markdownConverter) processInlineObject(out *bytes.Buffer, objRef *docs.InlineObjectElement) error {
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

func (dc *markdownConverter) processTextRun(out *bytes.Buffer, t *docs.TextRun) error {
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
	if t.TextStyle.SmallCaps {
		log.Printf("warning: ignoring smallcaps on %q", t.Content)
	}
	if t.TextStyle.BackgroundColor != nil {
		log.Printf("warning: ignoring background color on %q", t.Content)
	}
	if t.TextStyle.ForegroundColor != nil && t.TextStyle.Link == nil {
		log.Printf("warning: ignoring foreground color on %q", t.Content)
	}
	if t.TextStyle.Underline && t.TextStyle.Link == nil {
		log.Printf("warning: ignoring underlining on %q", t.Content)
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

		// in markdown, emphasis markers cannot be
		// separated from the content by whitespace
		leadingSpace, middle, trailingSpace := splitSpace(line)

		fmt.Fprint(out, leadingSpace)
		if len(middle) > 0 {
			fmt.Fprint(out, surround)
			fmt.Fprint(out, middle)
			fmt.Fprint(out, surround)
		}
		fmt.Fprint(out, trailingSpace)

		if link != nil {
			fmt.Fprintf(out, "](%s)", link.Url)
		}

		if i+1 < len(lines) {
			fmt.Fprintf(out, "\n")
		}
	}

	return nil
}
