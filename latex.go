package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"text/template"

	"google.golang.org/api/docs/v1"
)

type exportLatexArgs struct {
	Input        string `arg:"positional"`
	Output       string `arg:"-o,--output"`
	Bibliography string
	Template     string
}

func exportLatex(ctx context.Context, args *exportLatexArgs) error {
	// load the input document
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

	// convert the document to latex
	conv := latexConverter{
		doc: d.Doc,
	}

	// process the main body content
	var tex bytes.Buffer
	err = conv.process(&tex, d.Doc.Body.Content)
	if err != nil {
		return fmt.Errorf("error converting document body to latex: %w", err)
	}

	// process each footnote
	// for _, footnote := range d.Doc.Footnotes {
	// 	var footnoteMarkdown bytes.Buffer
	// 	err = conv.process(&footnoteMarkdown, footnote.Content)
	// 	if err != nil {
	// 		return fmt.Errorf("error converting footnote %s content to markdown: %w", footnote.FootnoteId, err)
	// 	}

	// 	fmt.Fprintf(&markdown, "[^%s]: ", footnote.FootnoteId)
	// 	for i, line := range strings.Split(footnoteMarkdown.String(), "\n") {
	// 		if i > 0 {
	// 			fmt.Fprint(&markdown, "    ") // multi-line footnotes in markdown must be indented
	// 		}

	// 		fmt.Fprintln(&markdown, line)
	// 	}

	// 	fmt.Fprint(&markdown, "\n") // make sure there is an empty line between each footnote
	// }

	// load the tex template
	tpl, err := template.ParseFiles("tex/template.tex")
	if err != nil {
		return fmt.Errorf("error parsing latex template: %w", err)
	}

	// pick a bibliography path
	bibPath := args.Bibliography
	if bibPath == "" {
		bibPath = "library.bib"
	}

	// execute the latex template
	type inputs struct {
		Title        string
		Subtitle     string
		Author       string
		Content      string
		Bibliography string
	}

	var out bytes.Buffer
	err = tpl.Execute(&out, inputs{
		Title:        "the title",
		Subtitle:     "the subtitle",
		Author:       "Kōshin",
		Content:      `\chapter{Foo}`,
		Bibliography: args.Bibliography,
	})
	if err != nil {
		return fmt.Errorf("error executing latex template: %w", err)
	}

	// write to output file or stdout
	if args.Output == "" {
		fmt.Println(out.String())
	} else {
		err = ioutil.WriteFile(args.Output, out.Bytes(), 0666)
		if err != nil {
			return fmt.Errorf("error writing to %s: %w", args.Output, err)
		}
	}

	return nil
}

type latexConverter struct {
	doc        *docs.Document
	imagePaths []string
	imageIndex int
	codeBlock  bytes.Buffer // text identified as lines of code
}

func (dc *latexConverter) process(out *bytes.Buffer, content []*docs.StructuralElement) error {
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

// flushCodeBlock writes any lines stored in dc.codeblock, or if there
// are no stored lines then it does nothing
func (dc *latexConverter) flushCodeBlock(out *bytes.Buffer) {
	if dc.codeBlock.Len() == 0 {
		return
	}

	fmt.Fprintln(out, "```")
	dc.codeBlock.WriteTo(out)
	fmt.Fprintln(out, "```")
	fmt.Fprintln(out)
	dc.codeBlock.Reset()
}

func (dc *latexConverter) processParagraph(out *bytes.Buffer, p *docs.Paragraph) error {
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

func (dc *latexConverter) processInlineObject(out *bytes.Buffer, objRef *docs.InlineObjectElement) error {
	// id := objRef.InlineObjectId
	// obj, ok := dc.doc.InlineObjects[id]
	// if !ok {
	// 	fmt.Println("warning: could not find inline object for id", id)
	// 	return nil
	// }

	// emb := obj.InlineObjectProperties.EmbeddedObject
	// switch {
	// case emb.ImageProperties != nil || emb.EmbeddedDrawingProperties != nil:
	// 	if dc.imageIndex >= len(dc.imageURLs) {
	// 		return fmt.Errorf("found %d images in zip but too many objects in the doc", len(dc.imageURLs))
	// 	}
	// 	fmt.Fprintf(out, "![%s](%s)", emb.Title, dc.imageURLs[dc.imageIndex])
	// 	dc.imageIndex++
	// case emb.LinkedContentReference != nil:
	// 	log.Println("warning: ignoring linked spreadsheet / chart")
	// }

	return nil
}

func (dc *latexConverter) processTextRun(out *bytes.Buffer, t *docs.TextRun) error {
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
		log.Printf("warning: ignoring background color (%q)", t.Content)
	}
	if t.TextStyle.ForegroundColor != nil {
		log.Printf("warning: ignoring foreground color (%q)", t.Content)
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
