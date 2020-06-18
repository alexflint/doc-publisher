package lesswrong

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// message captures just the generic parts of all messages
type message struct {
	Msg string `json:"msg"`
}

type connectMessage struct {
	Msg     string   `json:"msg"`
	Version string   `json:"version"`
	Support []string `json:"support"`
}

type loginUser struct {
	Email string `json:"email"`
}

type loginPassword struct {
	Digest    string `json:"digest"`
	Algorithm string `json:"algorithm"`
}

type loginParam struct {
	User     loginUser     `json:"user"`
	Password loginPassword `json:"password"`
}

// loginMessage is what we send to the server to authenticate
type loginMessage struct {
	Msg    string       `json:"msg"`
	Method string       `json:"method"`
	ID     string       `json:"id"`
	Params []loginParam `json:"params"`
}

type tokenResult struct {
	ID    string `json:"id"`
	Token string `json:"token"`
	Type  string `json:"type"`
}

type tokenError struct {
	Message string `json:"message"`
}

// tokenMessage is what we receive back from the server when we are authenticated
type tokenMessage struct {
	Msg    string       `json:"msg"`
	ID     string       `json:"id"`
	Result *tokenResult `json:"result"`
	Error  *tokenError  `json:"error"`
}

func marshalMessage(msg interface{}) ([]byte, error) {
	buf, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	buf2, err := json.Marshal([]string{string(buf)})
	if err != nil {
		return nil, err
	}

	return buf2, nil
}

// Auth represents the results of a successful authentication
type Auth struct {
	Token string
}

// authError is an auth together with an error
type authError struct {
	auth Auth
	err  error
}

const authTimeout = 10 * time.Second

// Authenticate connects to the lesswrong websocket, performs
// password-based authentication, and returns an auth token
func Authenticate(ctx context.Context, email, password string) (*Auth, error) {
	ctx, _ = context.WithTimeout(ctx, authTimeout)

	// connect to the websocket
	ws, resp, err := websocket.DefaultDialer.DialContext(
		ctx, "wss://www.lesswrong.com/sockjs/390/zlpzertg/websocket", http.Header{})
	if err != nil {
		return nil, fmt.Errorf("error dialing websocket: %w (HTTP response status was %v)", err, resp.Status)
	}
	defer ws.Close()

	// send a "connect" message
	connect := connectMessage{
		Msg:     "connect",
		Version: "1",
		Support: []string{"1", "pre2", "pre1"},
	}

	connectBuf, err := marshalMessage(connect)
	if err != nil {
		return nil, fmt.Errorf("error marshalling connect message: %w", err)
	}

	err = ws.WriteMessage(websocket.TextMessage, connectBuf)
	if err != nil {
		return nil, fmt.Errorf("error writing to websocket: %w", err)
	}

	// hash the password
	sum := sha256.Sum256([]byte(password))
	digest := hex.EncodeToString(sum[:])

	// create the login message
	login := loginMessage{
		Msg:    "method",
		Method: "login",
		ID:     "1",
		Params: []loginParam{{
			User: loginUser{
				Email: email,
			},
			Password: loginPassword{
				Digest:    digest,
				Algorithm: "sha-256",
			},
		}},
	}

	loginBuf, err := marshalMessage(login)
	if err != nil {
		return nil, fmt.Errorf("error marshalling the login message: %w", err)
	}

	err = ws.WriteMessage(websocket.TextMessage, loginBuf)
	if err != nil {
		return nil, fmt.Errorf("error sending the login message: %w", err)
	}

	// wait for a result from the server
	ch := make(chan authError)
	go func() {
		defer close(ch)
		for {
			_, buf, err := ws.ReadMessage()
			if err != nil {
				ch <- authError{err: fmt.Errorf("error reading from websocket: %w", err)}
				return
			}

			if len(buf) == 0 {
				log.Println("received an empty websocket message, ignoring")
				continue
			}

			// the first character tells us how to decode the message
			ctrl, sz := utf8.DecodeRune(buf)
			if ctrl != 'a' {
				log.Printf("received websocket message with control character %q, ignoring\n", ctrl)
				continue
			}

			buf = buf[sz:]

			// first unmarshal into an array of strings
			var parts []string
			err = json.Unmarshal(buf, &parts)
			if err != nil {
				ch <- authError{err: err}
				return
			}

			// now unmarshal each part on its own
			for _, part := range parts {
				var m message
				err = json.Unmarshal([]byte(part), &m)
				if err != nil {
					ch <- authError{err: err}
					return
				}

				if m.Msg != "result" {
					log.Printf("received a %q message, ignoring\n", m.Msg)
					continue
				}

				var msg tokenMessage
				err = json.Unmarshal([]byte(part), &msg)
				if err != nil {
					ch <- authError{err: err}
					return
				}

				log.Println(part)

				if msg.Error != nil {
					ch <- authError{err: fmt.Errorf("server said: %s", msg.Error.Message)}
					return
				}

				if msg.Result == nil {
					ch <- authError{err: fmt.Errorf("got a token message with neither result not error: %s", part)}
					return
				}

				ch <- authError{auth: Auth{Token: msg.Result.Token}}
			}
		}
	}()

	for {
		select {
		case x := <-ch:
			if x.err != nil {
				return nil, fmt.Errorf("authentication failed: %w", x.err)
			}
			return &x.auth, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

}
