package markdown

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexflint/doc-publisher/googledoc"
	"google.golang.org/api/docs/v1"
)

// FromGoogleDoc converts a google doc to markdown
func FromGoogleDoc(doc *docs.Document, imageURLByObjectID map[string]string) (string, error) {
	return FromGoogleDocSegment(doc, doc.Body.Content, imageURLByObjectID)
}

// FromGoogleDocSegment converts a part of a google doc to markdown
func FromGoogleDocSegment(doc *docs.Document, elements []*docs.StructuralElement, imageURLByObjectID map[string]string) (string, error) {
	// convert the document to markdown
	conv := markdownConverter{
		replace:            make(map[string]string),
		doc:                doc,
		imageURLByObjectID: imageURLByObjectID,
	}

	// process the main body content
	var markdown bytes.Buffer
	err := conv.process(&markdown, elements)
	if err != nil {
		return "", fmt.Errorf("error converting document body to markdown: %w", err)
	}

	// process the footnotes
	for _, footnoteID := range conv.footnotes {
		footnote, ok := doc.Footnotes[footnoteID]
		if !ok {
			fmt.Printf("warning: no content found for footnote %q referenced in document", footnoteID)
			continue
		}

		var footnoteMarkdown bytes.Buffer
		err = conv.process(&footnoteMarkdown, footnote.Content)
		if err != nil {
			return "", fmt.Errorf("error converting footnote %s content to markdown: %w", footnote.FootnoteId, err)
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

	var final strings.Builder
	final.Grow(markdown.Len())

	// first put the latex header in
	if conv.latexDefs.Len() > 0 {
		final.WriteString("$$\n")
		conv.latexDefs.WriteTo(&final)
		final.WriteString("$$\n\n")
	}

	// apply post-processing
	var emptylines int
	for _, line := range strings.Split(markdown.String(), "\n") {
		// note that whitespace on the left is important
		line = strings.TrimRightFunc(line, unicode.IsSpace)

		// drop sequences of two or more empty lines
		if len(line) == 0 {
			emptylines++
			if emptylines < 2 {
				final.WriteRune('\n')
			}
			continue
		}

		emptylines = 0

		// apply string-to-string replacements (used to rewrite \T1 to \Tone due to latex rules)
		for from, to := range conv.replace {
			line = strings.ReplaceAll(line, from, to)
		}

		final.WriteString(line + "\n")
	}

	return final.String(), nil
}

type markdownConverter struct {
	doc                *docs.Document
	imageURLByObjectID map[string]string
	codeBlock          bytes.Buffer // text identified as lines of code
	footnotes          []string     // footnote IDs processed by this converter
	latexDefs          bytes.Buffer
	replace            map[string]string // string replacements to apply to whole doc
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
			if !googledoc.IsMonospace(el.TextRun.TextStyle.WeightedFontFamily) {
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

	// print the blockquote prefix
	if p.ParagraphStyle.IndentStart != nil && p.ParagraphStyle.IndentStart.Magnitude > 0 && p.Bullet == nil {
		fmt.Fprintf(out, "> ")
	}

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
		fmt.Fprintf(out, "![%s](%s)", emb.Title, dc.imageURLByObjectID[id])
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
	if googledoc.IsMonospace(t.TextStyle.WeightedFontFamily) {
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

outer:
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}

		// lines that begin \newcommand or similar are treated as special latex blocks
		var cmd newcommand
		if newcommandPattern.Find(&cmd, line) {
			// latex symbols cannot contain digits so we rewrite \E0 to \Enought, \T1 to \Tone, and so forth
			fixed := fixLatexSymbol(cmd.Name)
			fmt.Fprintf(&dc.latexDefs, "\\newcommand{%s}{%s}\n", fixed, cmd.Value)
			if fixed != cmd.Name {
				dc.replace[cmd.Name] = fixed
			}
			continue outer
		}

		// write the beginning of a link in form [...TEXT...](...URL...)
		if link != nil {
			fmt.Fprint(out, "[")
		}

		// in markdown, emphasis markers cannot be
		// separated from the content by whitespace
		leadingSpace, middle, trailingSpace := splitSpace(line)

		fmt.Fprint(out, leadingSpace)
		if len(middle) > 0 {
			fmt.Fprint(out, surround)
			if err := dc.processLine(out, middle); err != nil {
				return err
			}
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

// processLine adds dollar signs around latex identifiers
func (dc *markdownConverter) processLine(out *bytes.Buffer, line string) error {
	var inLatex bool
	var pos int
	for len(line) > 0 {
		r, sz := utf8.DecodeRuneInString(line)
		if r == utf8.RuneError {
			return fmt.Errorf("error decoding rune from string at position %d", pos)
		}
		pos += sz
		line = line[sz:]

		word := unicode.IsNumber(r) || unicode.IsLetter(r)
		if !inLatex && r == rune('\\') {
			out.WriteString("$")
			inLatex = true
		} else if inLatex && !word {
			out.WriteString("$")
			inLatex = false
		}

		out.WriteRune(r)
	}

	if inLatex {
		out.WriteString("$")
	}

	return nil
}
