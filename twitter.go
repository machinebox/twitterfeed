package twitterfeed

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/garyburd/go-oauth/oauth"
)

// Tweet is a single tweet.
type Tweet struct {
	// Text is the body of the tweet.
	Text string
	// Terms is a list of matching terms in the text.
	Terms []string
}

// TweetReader reads tweets.
type TweetReader struct {
	consumerKey, consumerSecret, accessToken, accessSecret string
}

// NewTweetReader creates a new TweetReader with the given credentials.
func NewTweetReader(consumerKey, consumerSecret, accessToken, accessSecret string) *TweetReader {
	return &TweetReader{
		consumerKey:    consumerKey,
		consumerSecret: consumerSecret,
		accessToken:    accessToken,
		accessSecret:   accessSecret,
	}
}

// Run starts reading and returns a channel through which Tweet objects are sent.
// Use a cancel function or timeout on the context to terminate the reader.
func (r *TweetReader) Run(ctx context.Context, terms ...string) <-chan Tweet {
	tweetsChan := make(chan Tweet)
	var connLock sync.Mutex
	var conn net.Conn
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				connLock.Lock()
				defer connLock.Unlock()
				if conn != nil {
					conn.Close()
					conn = nil
				}
				netc, err := net.DialTimeout(netw, addr, 5*time.Second)
				if err != nil {
					return nil, err
				}
				conn = netc
				return netc, nil
			},
		},
	}
	creds := &oauth.Credentials{
		Token:  r.accessToken,
		Secret: r.accessSecret,
	}
	authClient := &oauth.Client{
		Credentials: oauth.Credentials{
			Token:  r.consumerKey,
			Secret: r.consumerSecret,
		},
	}
	go func() {
		// periodically close the connection to keep it fresh,
		// and if the context is done, close the connection and exit.
		// Closing the connection will cause the main loop to exit, which
		// in turn will check for a done context and abort, closing the channel.
		for {
			select {
			case <-ctx.Done():
				connLock.Lock()
				if conn != nil {
					conn.Close()
					conn = nil
				}
				connLock.Unlock()
				return
			case <-time.After(2 * time.Minute):
				connLock.Lock()
				if conn != nil {
					conn.Close()
					conn = nil
				}
				connLock.Unlock()
			}
		}
	}()
	go func() {
		// make the query and send all tweets into the channel
		defer close(tweetsChan)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				form := url.Values{"track": {strings.Join(terms, ",")}}
				formEnc := form.Encode()
				u, _ := url.Parse("https://stream.twitter.com/1.1/statuses/filter.json")
				req, err := http.NewRequest("POST", u.String(), strings.NewReader(formEnc))
				if err != nil {
					log.Println("creating filter request failed:", err)
					continue
				}
				req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "POST", u, form))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Content-Length", strconv.Itoa(len(formEnc)))
				resp, err := client.Do(req)
				if err != nil {
					log.Println("Error getting response:", err)
					continue
				}
				if resp.StatusCode != http.StatusOK {
					// this is a nice way to see what the error actually is:
					s := bufio.NewScanner(resp.Body)
					s.Scan()
					log.Println(s.Text())
					log.Println("StatusCode =", resp.StatusCode)
					continue
				}
				decoder := json.NewDecoder(resp.Body)
				func() {
					defer resp.Body.Close()
					for {
						var t Tweet
						if err := decoder.Decode(&t); err != nil {
							break
						}
						t.Terms = foundTerms(t.Text, terms...)
						tweetsChan <- t
					}
				}()
			}
		}
	}()
	return tweetsChan
}

// foundTerms searches text for any of the terms and returns a list
// of any that appear.
func foundTerms(text string, terms ...string) []string {
	text = strings.ToLower(text)
	var foundTerms []string
	for _, term := range terms {
		if strings.Contains(text, strings.ToLower(term)) {
			foundTerms = append(foundTerms, term)
		}
	}
	return foundTerms
}
