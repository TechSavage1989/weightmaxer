package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TechSavage1989/weightmaxer/internal/version"
)

// Kind is the outcome of one update check.
type Kind string

const (
	KindUpToDate    Kind = "up-to-date"
	KindAvailable   Kind = "available"
	KindNoRelease   Kind = "no-release"
	KindOffline     Kind = "offline"
	KindRateLimited Kind = "rate-limited"
	KindError       Kind = "error"
)

// Failed reports whether the check did not reach a trustworthy answer.
func (k Kind) Failed() bool { return k == KindOffline || k == KindRateLimited || k == KindError }

// Asset is one release download.
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// Release is the part of GitHub's release JSON Weightmaxer uses.
type Release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
	Version    string  `json:"-"`
}

func (r *Release) asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Result is what one check found.
type Result struct {
	Kind    Kind
	Release *Release
	Message string
	// PauseUntil asks the scheduler not to call GitHub again before this time
	// (Retry-After, an exhausted rate limit, or a nearly exhausted one).
	PauseUntil  time.Time
	NotModified bool
	At          time.Time
}

// Client talks to the release feed.
type Client struct {
	Config
	Current   string
	CachePath string
	Now       func() time.Time
}

type cache struct {
	ETag    string          `json:"etag"`
	Body    json.RawMessage `json:"body"`
	Checked time.Time       `json:"checked"`
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().Round(0)
}

func (c *Client) loadCache() cache {
	var k cache
	if b, err := os.ReadFile(c.CachePath); err == nil {
		_ = json.Unmarshal(b, &k)
	}
	return k
}

func (c *Client) saveCache(k cache) {
	if c.CachePath == "" {
		return
	}
	b, _ := json.Marshal(k)
	_ = os.MkdirAll(filepath.Dir(c.CachePath), 0o700)
	tmp := c.CachePath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, c.CachePath)
	}
}

// Check asks GitHub for the latest stable release, sending the cached ETag so
// an unchanged answer costs a 304 instead of a full response.
func (c *Client) Check(ctx context.Context) Result {
	now := c.now()
	res := Result{At: now}
	cur, err := version.Parse(c.Current)
	if err != nil {
		res.Kind, res.Message = KindError, "This is a development build, so updates are off."
		return res
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.APIBase, c.Owner, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		res.Kind, res.Message = KindError, err.Error()
		return res
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	k := c.loadCache()
	if k.ETag != "" && len(k.Body) > 0 {
		req.Header.Set("If-None-Match", k.ETag)
	}
	resp, err := c.client(30 * time.Second).Do(req)
	if err != nil {
		res.Kind = KindOffline
		res.Message = "Couldn't reach GitHub. Check your internet connection."
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			res.Message = "GitHub didn't answer in time."
		}
		return res
	}
	defer resp.Body.Close()
	res.PauseUntil = rateLimitPause(resp, now)

	var body []byte
	switch {
	case resp.StatusCode == http.StatusNotModified && len(k.Body) > 0:
		body, res.NotModified = k.Body, true
		k.Checked = now
		c.saveCache(k)
	case resp.StatusCode == http.StatusOK:
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxAPISize+1))
		if err != nil || len(body) > maxAPISize {
			res.Kind, res.Message = KindError, "GitHub sent an unexpected answer."
			return res
		}
		c.saveCache(cache{ETag: resp.Header.Get("ETag"), Body: body, Checked: now})
	case resp.StatusCode == http.StatusNotFound:
		res.Kind, res.Message = KindNoRelease, "No Weightmaxer release has been published yet."
		return res
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		if res.PauseUntil.IsZero() && resp.StatusCode == http.StatusTooManyRequests {
			res.PauseUntil = now.Add(15 * time.Minute)
		}
		if !res.PauseUntil.IsZero() {
			res.Kind = KindRateLimited
			res.Message = "GitHub's limit for update checks from your network was reached."
			return res
		}
		fallthrough
	default:
		res.Kind = KindError
		res.Message = fmt.Sprintf("GitHub answered with an error (HTTP %d).", resp.StatusCode)
		return res
	}

	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		res.Kind, res.Message = KindError, "GitHub sent release details Weightmaxer couldn't read."
		return res
	}
	if rel.Draft || rel.Prerelease {
		res.Kind, res.Message = KindError, "The latest release is not a stable release."
		return res
	}
	latest, err := version.Parse(rel.Tag)
	if err != nil {
		res.Kind, res.Message = KindError, fmt.Sprintf("The latest release has an unexpected version (%q).", rel.Tag)
		return res
	}
	rel.Version = latest.String()
	res.Release = &rel
	if latest.Compare(cur) <= 0 {
		res.Kind = KindUpToDate
		return res
	}
	if rel.asset(AssetName) == nil || rel.asset(SumsName) == nil {
		res.Kind = KindError
		res.Message = fmt.Sprintf("Version %s is published but its Windows download isn't ready yet.", rel.Version)
		return res
	}
	res.Kind = KindAvailable
	return res
}

// rateLimitPause reads Retry-After and GitHub's rate-limit headers. When fewer
// than five unauthenticated requests remain it pauses until the limit resets,
// leaving the rest for other copies on the same network.
func rateLimitPause(resp *http.Response, now time.Time) time.Time {
	if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
		if s, err := strconv.Atoi(ra); err == nil && s >= 0 {
			return now.Add(time.Duration(min(s, 6*3600)) * time.Second)
		}
		if t, err := http.ParseTime(ra); err == nil && t.After(now) {
			return minTime(t, now.Add(6*time.Hour))
		}
	}
	rem, err1 := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	reset, err2 := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err1 == nil && err2 == nil && rem < 5 {
		t := time.Unix(reset, 0)
		if t.After(now) {
			return minTime(t, now.Add(2*time.Hour))
		}
	}
	return time.Time{}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
