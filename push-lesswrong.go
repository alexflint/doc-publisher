package main

import (
	"context"
	"log"

	"github.com/alexflint/doc-publisher/lesswrong"
)

type pushToLesswrongArgs struct {
	Password string
}

// very unfinished...
func pushToLesswrong(ctx context.Context, args *pushToLesswrongArgs) error {
	if args.Password != "" {
		// authenticate to lesswrong
		auth, err := lesswrong.Authenticate(ctx, "alex.flint@gmail.com", args.Password)
		if err != nil {
			return err
		}

		log.Println("got auth token:", auth.Token)

		lw, err := lesswrong.New()
		if err != nil {
			return err
		}

		lw.Auth = auth
	}

	return nil
}
