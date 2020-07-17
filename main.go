package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
)

type pullArgs struct {
	GoogleDoc *pullGoogleDocArgs `arg:"subcommand"`
}

type pushArgs struct {
	LessWrong *pushToLesswrongArgs `arg:"subcommand"`
}

type args struct {
	Pull *pullArgs `arg:"subcommand"`
	Push *pushArgs `arg:"subcommand"`
}

func main() {
	ctx := context.Background()

	var args args
	p := arg.MustParse(&args)

	var err error

	switch {
	case args.Pull != nil:
		switch {
		case args.Pull.GoogleDoc != nil:
			err = pullGoogleDoc(ctx, args.Pull.GoogleDoc)
		default:
			p.Fail("pull requires a subcommand")
		}

	case args.Push != nil:
		switch {
		case args.Push.LessWrong != nil:
			err = pushToLesswrong(ctx, args.Push.LessWrong)
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
