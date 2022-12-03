package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
)

type fetchArgs struct {
	GoogleDoc *fetchGoogleDocArgs `arg:"subcommand"`
}

type exportArgs struct {
	Markdown *exportMarkdownArgs `arg:"subcommand"`
	Latex    *exportLatexArgs    `arg:"subcommand"`
}

type pushArgs struct {
	LessWrong *pushToLesswrongArgs `arg:"subcommand"`
	GoogleDoc *pushGoogleDocArgs   `arg:"subcommand"`
	Image     *pushImageArgs       `arg:"subcommand"`
}

type args struct {
	Fetch  *fetchArgs  `arg:"subcommand"`
	Export *exportArgs `arg:"subcommand"`
	Push   *pushArgs   `arg:"subcommand"`
}

func main() {
	ctx := context.Background()

	var args args
	p := arg.MustParse(&args)

	var err error

	switch {
	case args.Fetch != nil:
		switch {
		case args.Fetch.GoogleDoc != nil:
			err = fetchGoogleDoc(ctx, args.Fetch.GoogleDoc)
		default:
			p.Fail("pull requires a subcommand")
		}

	case args.Export != nil:
		switch {
		case args.Export.Markdown != nil:
			err = exportMarkdown(ctx, args.Export.Markdown)
		case args.Export.Latex != nil:
			err = exportLatex(ctx, args.Export.Latex)
		default:
			p.Fail("export requires a subcommand")
		}

	case args.Push != nil:
		switch {
		case args.Push.LessWrong != nil:
			err = pushToLesswrong(ctx, args.Push.LessWrong)
		case args.Push.GoogleDoc != nil:
			err = pushGoogleDoc(ctx, args.Push.GoogleDoc)
		case args.Push.Image != nil:
			err = pushImage(ctx, args.Push.Image)
		default:
			p.Fail("push requires a subcommand")
		}

	default:
		p.Fail("you must specify a subcommand")
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
