package lesswrong

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/machinebox/graphql"
)

type userResponse struct {
	User struct {
		Result *User
	}
}

// User represents information about a lesswrong
type User struct {
	Username string
	Email    string
	Bio      string
}

// Client makes API calls to lesswrong
type Client struct {
	graphql *graphql.Client
	Auth    *Auth
}

// New creates a new client
func New() (*Client, error) {
	c := Client{
		graphql: graphql.NewClient("https://www.lesswrong.com/graphql?"),
	}

	c.graphql.Log = func(s string) {
		log.Println(s)
	}

	return &c, nil
}

// User fetches information about a user
func (c *Client) User(ctx context.Context, username string) (*User, error) {
	// make a request
	req := graphql.NewRequest(`
	query($username: String!) {
		user(input: {selector: {slug: $username}}) {
			result {
				username
				email
				bio
			}
		}
	}`)

	req.Var("username", username)

	// run it and capture the response
	var res userResponse
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return nil, fmt.Errorf("error performing graphql query: %w", err)
	}

	return res.User.Result, nil
}

// CreatePost creates a new post
func (c *Client) CreatePost(ctx context.Context) error {
	// make a request
	req := graphql.NewRequest(`
	mutation {
		createPost(data: {
			title: "test from golang",
			submitToFrontpage: true,
			draft: true,
			meta: false,
			isEvent: false,
			types: [],
			moderationStyle: "easy-going",
			contents: {
				originalContents: {
					type: "markdown",
					data: "content from golang",
				},
				updateType: "minor",
			}
		}) {
			data {
				url,
				author,
			}
		}
	}`)

	if c.Auth != nil {
		req.Header.Set("Authorization", c.Auth.Token)
	}

	// run it and capture the response
	var res json.RawMessage
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return fmt.Errorf("error performing graphql query: %w", err)
	}

	fmt.Println(string(res))

	return nil
}
