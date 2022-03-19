package main

import (
	"context"
	"fmt"

	"github.com/alexflint/doc-publisher/lesswrong"
)

type pushToLesswrongArgs struct {
	Password string
}

// very unfinished...
func pushToLesswrong(ctx context.Context, args *pushToLesswrongArgs) error {
	if args.Password != "" {
		// authenticate to lesswrong
		lw, err := lesswrong.NewClient(ctx, "alex.flint@gmail.com", args.Password)
		if err != nil {
			return err
		}

		me, err := lw.User(ctx, "alex.flint@gmail.com")
		fmt.Printf("got user info: %#v\n", me)
	}

	return nil
}
