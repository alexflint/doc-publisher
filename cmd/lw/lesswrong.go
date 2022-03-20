// a utility for testing REST interactions with lesswrong

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alexflint/doc-publisher/lesswrong"
	"github.com/alexflint/go-arg"
)

func main() {
	ctx := context.Background()

	var args struct {
		Username string `arg:"required"`
		Password string `arg:"required"`
		Title    string
		Content  string
	}
	args.Title = "test title"
	args.Content = "test content"
	arg.MustParse(&args)

	client, err := lesswrong.NewClient(ctx, args.Username, args.Password)
	if err != nil {
		fmt.Printf("error authenticating: %v\n", err)
		os.Exit(1)
	}

	r, err := client.CreatePost(ctx, lesswrong.CreatePostRequest{
		Title:   args.Title,
		Content: args.Content,
		Draft:   true,
	})
	if err != nil {
		fmt.Printf("error creating lesswrong post: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("created post: " + r.URL)
}
