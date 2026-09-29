package webui

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aex/internal/httpx"
)

// The cat widget's picture, from Cat as a service (https://cataas.com). A widget has no network,
// so the picture comes to it as a data: URL.
const (
	catURL     = "https://cataas.com/cat"
	catTimeout = 20 * time.Second
	maxCat     = 8 << 20 // bytes: a bigger picture is not shown
)

// CatReply is the cat API's result.
type CatReply struct {
	URL string `json:"url"` // data:image/...;base64,...
}

// catAPI fetches a random cat picture. cataas.com sometimes answers 500, so it tries twice.
func catAPI(map[string]any) (any, error) {
	var err error
	for range 2 {
		var url string
		if url, err = fetchCat(); err == nil {
			return CatReply{URL: url}, nil
		}
	}
	return nil, err
}

func fetchCat() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), catTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", catURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "image/*")
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cataas.com: %s", resp.Status)
	}
	mime := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("cataas.com: not a picture: %q", mime)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCat+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxCat {
		return "", fmt.Errorf("cataas.com: picture bigger than %d MB", maxCat>>20)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
