package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/alexflint/doc-publisher/googledoc"
)

type exportLatexArgs struct {
	Input        string `arg:"positional"`
	Output       string `arg:"-o,--output"`
	Bibliography string
	Template     string
}

func exportLatex(ctx context.Context, args *exportLatexArgs) error {
	// load the input document
	_, err := googledoc.Load(args.Input)
	if err != nil {
		return err
	}

	// convert the document to latex (TODO!!)
	var tex bytes.Buffer

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
		Content:      tex.String(),
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
