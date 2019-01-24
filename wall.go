package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	beeline "github.com/honeycombio/beeline-go"
	"github.com/honeycombio/beeline-go/wrappers/hnynethttp"

	// Included for debugging purposes
	"github.com/honeycombio/beeline-go/propagation"
	"github.com/honeycombio/beeline-go/trace"
)

var (
	contents = []string{"First post!"}

	usernameRegexp = regexp.MustCompile(`@([a-z0-9_]+)`)
	hashtagRegexp  = regexp.MustCompile(`#([a-z0-9]+)`)
)

const (
	HONEYCOMB_API_KEY = "e841bedc1eb9ffd93c4c958b74e2d877"
	HONEYCOMB_DATASET = "dryrun-workshop"

	linkToHashtag = `<a href="https://twitter.com/hashtag/$1">#$1</a>`
	linkToProfile = `<a href="%s">%s</a>`
	persistURL    = "https://p3m11fv104.execute-api.us-east-1.amazonaws.com/dev/"
)

// = HANDLER =======================================================
// Returns the current contents of our "wall".
// =================================================================
func list(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, fmt.Sprint(strings.Join(contents, "<br />\n"),
		`<br /><br /><a href="/message">+ New Post</a>`))
}

// = HANDLER =======================================================
// Returns a simple HTML form for posting a new message to our wall.
// =================================================================
func newMessage(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, `<form method="POST" action="/">
		<input type="text" autofocus name="message" /><input type="submit" />
	</form>`)
}

// = HANDLER =======================================================
// Processes a string from the client and saves the message contents.
// =================================================================
func write(w http.ResponseWriter, r *http.Request) {
	body := strings.TrimSpace(r.FormValue("message"))

	ch := make(chan float64)
	go analyze(r.Context(), body, ch)

	body = twitterize(r.Context(), body)

	// // = CHECKPOINT 4: UNCOMMENT THIS BLOCK =======================
	// // Let's persist our wall contents! POST each message to a
	// // third-party service (in this case, a Lambda function).
	// post(r.Context(), persistURL, body)
	// // ============================================================

	sentiment := <-ch
	beeline.AddField(r.Context(), "sentiment", sentiment)
	if sentiment >= 0.2 {
		body = fmt.Sprint("<b>", body, "</b>")
	} else if sentiment <= -0.2 {
		body = fmt.Sprint("<i>", body, "</i>")
	}

	contents = append(contents, body)

	http.Redirect(w, r, "/", http.StatusFound)
}

// = HELPER ========================================================
// Identifies hashtags and Twitter handle-like strings. Replaces
// hashtags with links to a Twitter search for the found hashtag and
// replaces handle-like strings with links to the Twitter profile
// *if* a valid profile is found.
// =================================================================
func twitterize(ctx context.Context, content string) string {
	newContent := hashtagRegexp.ReplaceAllString(content, linkToHashtag)

	matches := usernameRegexp.FindAllString(newContent, -1)
	for _, handle := range matches {
		// // = CHECKPOINT 2, PART 1: UNCOMMENT THIS BLOCK ===============
		// _, span := beeline.StartSpan(ctx, "check_twitter")
		// // ============================================================
		profile := "https://twitter.com/" + handle[1:]
		resp, _ := http.Get(profile)
		if resp.StatusCode == http.StatusOK {
			newContent = strings.Replace(newContent, handle, fmt.Sprintf(linkToProfile, profile, handle), 1)
		}
		// // = CHECKPOINT 2, PART 2: UNCOMMENT THIS BLOCK ===============
		// span.AddField("app.twitter.handle", handle)
		// span.AddField("app.twitter.response_status", resp.StatusCode)
		// span.Send()
		// // ============================================================
	}

	return newContent
}

// = HELPER ========================================================
// Calls out to a second service of ours (which may or may not be
// live) to perform some further analysis on the post content.
// =================================================================
func analyze(ctx context.Context, content string, ch chan float64) {
	// This calls out to a second service
	resp, err := post(ctx, "http://localhost:8088", content)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		if b, err := ioutil.ReadAll(resp.Body); err == nil {
			f, _ := strconv.ParseFloat(string(b), 64)
			ch <- f
			return
		}
	}
	ch <- 0.0
}

// = HELPER ========================================================
// Replaces our vanilla `http.Post` so that we can customize the
// behavior of our HTTP client when calling an external service.
// =================================================================
func post(ctx context.Context, url, content string) (*http.Response, error) {
	client := &http.Client{
		// // = CHECKPOINT 3: UNCOMMENT THIS BLOCK =======================
		// // http.DefaultTransport is the default interface wrapping the ability
		// // to execute a single HTTP transaction. As a result, it's a convenient
		// // place for a wrapper looking to capture metadata about executed HTTP
		// // requests.
		// Transport: hnynethttp.WrapRoundTripper(http.DefaultTransport),
		// // ============================================================
		Timeout: 10 * time.Second,
	}

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(content))
	return client.Do(req.WithContext(ctx))
}

// = HELPER ========================================================
// Simplifies returning HTML responses.
// =================================================================
func renderHTML(w http.ResponseWriter, content string) {
	header := w.Header()
	header.Add("Content-Type", "text/html")
	fmt.Fprintln(w, content)
}

// = HELPER ========================================================
// Wraps HTTP handlers to output evidence of HTTP calls + trace IDs
// to STDOUT for debugging.
// =================================================================
func withTracing(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		span := trace.GetSpanFromContext(r.Context())
		prop, _ := propagation.UnmarshalTraceContext(span.SerializeHeaders())
		logrus.WithField("method", r.Method).
			WithField("path", r.URL.Path).
			WithField("trace_id", prop.TraceID).Infoln("Handling request")
		next.ServeHTTP(w, r)
	}
}

// = MAIN ==========================================================
// Entry point for our "wall" service.
// =================================================================
func main() {
	beeline.Init(beeline.Config{
		WriteKey:    HONEYCOMB_API_KEY,
		Dataset:     HONEYCOMB_DATASET,
		ServiceName: "wall",
	})
	defer beeline.Close()

	http.HandleFunc("/favicon.ico", http.NotFound)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// // = CHECKPOINT 1: UNCOMMENT THIS BLOCK =======================
		// // Add some way to identify these requests as coming from you!
		// // AddFieldToTrace ensures that this field will be populated on
		// // *all* spans in the trace, not just the currently active one.
		// beeline.AddFieldToTrace(r.Context(), "username", "YOUR_USERNAME_HERE")
		// // ============================================================
		if r.Method == http.MethodPost {
			withTracing(write)(w, r)
		} else {
			withTracing(list)(w, r)
		}
	})
	http.HandleFunc("/message", withTracing(newMessage))

	logrus.Infoln("Serving on localhost:8080...")
	logrus.Fatalln(http.ListenAndServe(":8080", hnynethttp.WrapHandler(http.DefaultServeMux)))
}
