package imgur

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

// Client provides access to the imgur upload API
type Client struct {
	APIKey string
	http   *http.Client
}

type uploadResponseData struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Link   string `json:"link"`
}

type uploadResponse struct {
	Success bool               `json:"success"`
	Status  int                `json:"status"`
	Data    uploadResponseData `json:"data"`
}

// New creates a new client
func New(apikey string) *Client {
	return &Client{
		APIKey: apikey,
		http:   http.DefaultClient,
	}
}

// Upload pushes an image to imgur and returns its public URL
func (c *Client) Upload(ctx context.Context, buf []byte) (string, error) {
	form := make(url.Values)
	form.Set("image", base64.StdEncoding.EncodeToString(buf))

	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.imgur.com/3/upload", body)
	if err != nil {
		return "", err
	}
	fmt.Printf("authorizing to imgur with %s\b", c.APIKey)
	req.Header.Set("Authorization", "Client-ID "+c.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(buf))
		return "", fmt.Errorf("server said: %s", resp.Status)
	}

	var r uploadResponse
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return "", fmt.Errorf("error decode imgur response payload: %w", err)
	}

	return r.Data.Link, nil
}
