package lesswrong

import (
	"context"
	"fmt"
	"net/http"

	"github.com/machinebox/graphql"
)

// Client makes API calls to lesswrong
type Client struct {
	graphql *graphql.Client
	auth    *Auth
}

// NewClient authenticates with lesswrong and returns a client
func NewClient(ctx context.Context, username, password string) (*Client, error) {
	c := Client{
		graphql: graphql.NewClient("https://www.lesswrong.com/graphql?"),
	}

	// c.graphql.Log = func(s string) {
	// 	log.Println("graphql said: ", s)
	// }

	r, err := c.login(ctx, loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("lesswrong authentication failed: %w", err)
	}

	c.auth = &Auth{Token: r.Token}

	return &c, nil
}

func (c *Client) createRequest(query string) *graphql.Request {
	req := graphql.NewRequest(query)
	if c.auth != nil {
		ck := http.Cookie{Name: "loginToken", Value: c.auth.Token}
		req.Header.Set("Cookie", ck.String()+";")
	}
	return req
}

// User represents information about a lesswrong user
type User struct {
	Username string
	Email    string
	Bio      string
}

type userContainer struct {
	User struct {
		Result *User
	}
}

// User fetches information about a user
func (c *Client) User(ctx context.Context, username string) (*User, error) {
	// make a request
	req := c.createRequest(`
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
	var res userContainer
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return nil, fmt.Errorf("error performing graphql query: %w", err)
	}

	return res.User.Result, nil
}

type loginRequest struct {
	Username string
	Password string
}

type loginResponse struct {
	Token string `json:"token"`
}

type loginResponseContainer struct {
	Login loginResponse `json:"login"`
}

// CreatePost creates a new post
func (c *Client) login(ctx context.Context, r loginRequest) (*loginResponse, error) {
	// construct a graphQL request
	req := graphql.NewRequest(`
	mutation($username: String!, $password: String!) {
		login(username: $username, password: $password) {
			token
		}
	}`)
	req.Var("username", r.Username)
	req.Var("password", r.Password)

	// send the request
	var res loginResponseContainer
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return nil, fmt.Errorf("error performing graphql query: %w", err)
	}
	return &res.Login, nil
}

type CreatePostRequest struct {
	Title   string // title of the post
	Content string // markdown-formatted post contents
}

type CreatePostResponse struct {
	URL string `json:"linkUrl"`
}

type createPostContainer struct {
	CreatePost struct {
		Data *CreatePostResponse `json:"data"`
	} `json:"createPost"`
}

type postContents struct {
	UpdateType       string               `json:"updateType"` // set to "minor"
	OriginalContents postOriginalContents `json:"originalContents"`
}

type postOriginalContents struct {
	Type string `json:"type"` // set to "markdown"
	Data string `json:"data"` // the content of the post, in markdown
}

// CreatePost creates a new post
func (c *Client) CreatePost(ctx context.Context, r CreatePostRequest) (*CreatePostResponse, error) {
	req := c.createRequest(`
	mutation($title:String!, $contents:JSON!) {
		createPost(data: {
			title: $title,
			submitToFrontpage: true,
			draft: true,
			meta: false,
			isEvent: false,
			types: [],
			moderationStyle: "easy-going",
			contents: $contents,
		}) {
			data {
				linkUrl,
			}
		}
	}`)

	// The "contents" field below above is a raw JSON blob, so we have to make the whole thing a variable
	contents := postContents{
		UpdateType: "minor",
		OriginalContents: postOriginalContents{
			Type: "markdown",
			Data: r.Content,
		},
	}

	req.Var("contents", contents)
	req.Var("title", r.Title)

	// run it and capture the response
	var res createPostContainer
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return nil, fmt.Errorf("error performing graphql query: %w", err)
	}
	return res.CreatePost.Data, nil
}
