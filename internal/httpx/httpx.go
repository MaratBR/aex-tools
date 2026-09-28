// Package httpx has request/response handling shared by the API clients: anything unexpected
// (status, non-JSON body, wrong shape) is dumped to stderr with status, URL, relevant headers and
// a capped body, then returned as an error.
package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"aex/internal/ui"
)

// Client is shared by the API clients.
var Client = &http.Client{Timeout: 2 * time.Minute}

const maxDumpBytes = 300 * 1024

var relevantHeaders = []*regexp.Regexp{
	regexp.MustCompile(`^content-(type|length|encoding)$`),
	regexp.MustCompile(`^(date|server|location|retry-after|www-authenticate|allow|via)$`),
	regexp.MustCompile(`request-?id|trace-?id|correlation-?id|^cf-ray$|^x-arequestid$`),
	regexp.MustCompile(`ratelimit`),
	regexp.MustCompile(`^x-(seraph-loginreason|ausername|cache|error.*)$`),
}

// Keep cookie names and attributes, hide values.
var cookieValue = regexp.MustCompile(`^([^=;]+)=[^;]*`)

// NewRequest builds a request sending body as JSON (when not nil) and accepting JSON.
func NewRequest(method, url string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Dump prints a response to stderr.
func Dump(label string, res *http.Response, body []byte, reason string) {
	c := ui.Err
	lines := []string{
		c.Red(fmt.Sprintf("--- %s: %s ---", label, reason)),
		c.Bold("HTTP " + res.Status),
		c.Dim("URL:") + " " + res.Request.URL.String(),
	}
	names := make([]string, 0, len(res.Header))
	for name := range res.Header {
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "set-cookie" || !slices.ContainsFunc(relevantHeaders, func(re *regexp.Regexp) bool { return re.MatchString(name) }) {
			continue
		}
		lines = append(lines, c.Dim(name+":")+" "+strings.Join(res.Header.Values(name), ", "))
	}
	for _, cookie := range res.Header.Values("Set-Cookie") {
		lines = append(lines, c.Dim("set-cookie:")+" "+cookieValue.ReplaceAllString(cookie, "$1=<redacted>"))
	}

	lines = append(lines, "")
	if len(body) == 0 {
		lines = append(lines, c.Dim("<empty body>"))
	} else {
		lines = append(lines, strings.ToValidUTF8(string(body[:min(len(body), maxDumpBytes)]), ""))
	}
	if len(body) > maxDumpBytes {
		lines = append(lines, c.Yellow(fmt.Sprintf("[truncated: showing %d of %d bytes]", maxDumpBytes, len(body))))
	}
	lines = append(lines, c.Red(fmt.Sprintf("--- end %s ---", label)))
	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}

// ReadJSON reads and closes a response and decodes its JSON body into T (zero T for an empty body).
// Dumps and fails unless the status is in expect (nil: any 2xx), the body decodes into T, and
// validate (when not nil) passes.
func ReadJSON[T any](label string, res *http.Response, expect []int, validate func(T) bool) (T, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		body = []byte("<failed to read body: " + err.Error() + ">")
	}
	var data T
	fail := func(reason string) (T, error) {
		Dump(label, res, body, reason)
		return data, fmt.Errorf("%s: %s (HTTP %d, details above)", label, reason, res.StatusCode)
	}

	ok := res.StatusCode >= 200 && res.StatusCode < 300
	if expect != nil {
		ok = slices.Contains(expect, res.StatusCode)
	}
	if !ok {
		return fail("unexpected status")
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) {
				return fail("response is not JSON")
			}
			return fail("unexpected response shape")
		}
	}
	if validate != nil && !validate(data) {
		return fail("unexpected response shape")
	}
	return data, nil
}

// Discard drains and closes a response body so the connection can be reused.
func Discard(res *http.Response) {
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
}
