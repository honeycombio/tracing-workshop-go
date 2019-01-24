package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/beeline-go"
	"github.com/honeycombio/beeline-go/wrappers/hnynethttp"
)

const (
	HONEYCOMB_API_KEY = "e841bedc1eb9ffd93c4c958b74e2d877"
	HONEYCOMB_DATASET = "dryrun-workshop"

	GCP_URL_TEMPLATE = "https://language.googleapis.com/v1/documents:analyzeSentiment?key=%s"
	GCP_API_KEY      = "AIzaSyAXdTnJgZu0oMUb4I3VN2Mepx_KpXBB5RA"
)

// = HANDLER =======================================================
// Calls out to various APIs (in this case, Google's Natural Language
// API) to perform a bit of analysis on client-provided strings.
// Returns a float representing any detected sentiment.
// =================================================================
func parse(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		beeline.AddField(r.Context(), "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body := fmt.Sprintf(`{"document":{
		"type":"PLAIN_TEXT",
		"content":"%s"
	}, "encodingType":"UTF8"}`, b)

	resp, err := postGCP(r.Context(), body)
	if err != nil {
		beeline.AddField(r.Context(), "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	nlpResponse := struct {
		Sentiment struct {
			Magnitude float64 `json:"magnitude"`
			Score     float64 `json:"score"`
		} `json:"documentSentiment"`
	}{}
	if err = json.NewDecoder(resp.Body).Decode(&nlpResponse); err != nil {
		beeline.AddField(r.Context(), "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, nlpResponse.Sentiment.Score)
}

// = HELPER ========================================================
// A specialized version of the post helper from wall.go, explicitly
// for wrapping POST calls to GCP's Sentiment Analysis API.
// =================================================================
func postGCP(ctx context.Context, content string) (*http.Response, error) {
	client := &http.Client{
		Transport: hnynethttp.WrapRoundTripper(http.DefaultTransport),
	}
	url := fmt.Sprintf(GCP_URL_TEMPLATE, GCP_API_KEY)
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(content))
	req.Header.Add("Content-Type", "application/json")
	return client.Do(req.WithContext(ctx))
}

// = MAIN ==========================================================
// Entry point for our users service.
// =================================================================
func main() {
	beeline.Init(beeline.Config{
		WriteKey:    HONEYCOMB_API_KEY,
		Dataset:     HONEYCOMB_DATASET,
		ServiceName: "analysis",
	})
	defer beeline.Close()

	http.HandleFunc("/", parse)

	logrus.Infoln("Serving on localhost:8088...")
	logrus.Fatalln(http.ListenAndServe(":8088", hnynethttp.WrapHandler(http.DefaultServeMux)))
}
