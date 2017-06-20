# twitterfeed
Go channel providing live tweets containing search terms.

The following code reads all tweets containing `"monkey"` for five seconds:

```go
ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

r := twitterfeed.NewTweetReader("consumerKey", "consumerSecret", "accessToken", "accessSecret")
for tweet := range r.Run(ctx, "monkey") {
	log.Println(tweet)
}

log.Println("done")
```
