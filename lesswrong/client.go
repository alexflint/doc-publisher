package lesswrong

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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
	auth    *Auth
}

// New creates a new client
func NewClient(ctx context.Context, username, password string) (*Client, error) {
	c := Client{
		graphql: graphql.NewClient("https://www.lesswrong.com/graphql?"),
	}

	c.graphql.Log = func(s string) {
		log.Println("graphql said: ", s)
	}

	loginResp, err := c.login(ctx, loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("lesswrong authentication failed: %w", err)
	}

	log.Println("lesswrong login token: ", loginResp.Token)
	c.auth = &Auth{Token: loginResp.Token}

	return &c, nil
}

func (c *Client) createRequest(query string) *graphql.Request {
	req := graphql.NewRequest(query)
	if c.auth != nil {
		ck := http.Cookie{Name: "loginToken", Value: c.auth.Token}
		log.Println("setting cookie: ", ck.String())
		req.Header.Set("Cookie", ck.String()+";")
	}
	return req
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
	var res userResponse
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return nil, fmt.Errorf("error performing graphql query: %w", err)
	}

	return res.User.Result, nil
}

type loginResponseContainer struct {
	Login loginResponse `json:"login"`
}

type loginResponse struct {
	Token string `json:"token"`
}

type loginRequest struct {
	Username string
	Password string
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

	log.Printf("received from lesswrong for login: %#v", res)

	return &res.Login, nil
}

type CreatePostRequest struct {
	Title     string
	Content   string // markdown
	Draft     bool
	Frontpage bool
}

// CreatePost creates a new post
func (c *Client) CreatePost(ctx context.Context, r CreatePostRequest) error {
	req := c.createRequest(`
	mutation($title:String!, $content:String!) {
		createPost(data: {
			title: $title,
			submitToFrontpage: true,
			draft: true,
			meta: false,
			isEvent: false,
			types: [],
			moderationStyle: "easy-going",
			contents: {
				originalContents: {
					type: "markdown",
					data: $content,
				},
				updateType: "minor",
			}
		}) {
			data {
				url,
				pageUrl,
				linkUrl,
				author,
				user,
			}
		}
	}`)

	req.Var("title", r.Title)
	req.Var("content", r.Content)

	log.Println("CreatePost cookie:", req.Header.Get("Cookie"))

	// run it and capture the response
	var res json.RawMessage
	if err := c.graphql.Run(ctx, req, &res); err != nil {
		return fmt.Errorf("error performing graphql query: %w", err)
	}

	log.Println("received from lesswrong:", string(res))

	return nil
}
