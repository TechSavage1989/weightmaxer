package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Progress reports bytes received so far out of total.
type Progress func(done, total int64)

// Download fetches the release's Windows zip into dir and confirms its SHA-256
// against the release's SHA256SUMS.txt and, when GitHub provides one, the
// asset digest. A matching hash proves the file is the one published in this
// repository's release; it is not a publisher code-signing signature.
func (c *Client) Download(ctx context.Context, rel *Release, dir string, progress Progress) (string, error) {
	zipAsset, sumsAsset := rel.asset(AssetName), rel.asset(SumsName)
	if zipAsset == nil || sumsAsset == nil {
		return "", errors.New("the release has no Windows download")
	}
	for _, a := range []*Asset{zipAsset, sumsAsset} {
		want := c.DownloadPrefix + "v" + rel.Version + "/" + a.Name
		if a.URL != want {
			return "", fmt.Errorf("unexpected download address for %s", a.Name)
		}
		if err := c.checkURL(a.URL); err != nil {
			return "", err
		}
	}
	if zipAsset.Size <= 0 || zipAsset.Size > maxZipSize {
		return "", fmt.Errorf("the download has an unexpected size (%d bytes)", zipAsset.Size)
	}
	sums, err := c.fetch(ctx, sumsAsset.URL, maxSumsSize)
	if err != nil {
		return "", fmt.Errorf("couldn't download the checksum list: %w", err)
	}
	want, err := sumFor(sums, AssetName)
	if err != nil {
		return "", err
	}
	if d := strings.ToLower(zipAsset.Digest); d != "" && d != "sha256:"+want {
		return "", errors.New("GitHub's digest for the download doesn't match the checksum list")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(dir, fmt.Sprintf("Weightmaxer-%s-win-x64.zip", rel.Version))
	part := final + ".part"
	if err := c.fetchTo(ctx, zipAsset.URL, part, zipAsset.Size, want, progress); err != nil {
		os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, final); err != nil {
		os.Remove(part)
		return "", err
	}
	return final, nil
}

func (c *Client) get(ctx context.Context, raw string) (*http.Response, error) {
	if err := c.checkURL(raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.client(15 * time.Minute).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("the server answered HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) fetch(ctx context.Context, raw string, limit int64) ([]byte, error) {
	resp, err := c.get(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("the file is larger than expected")
	}
	return b, nil
}

func (c *Client) fetchTo(ctx context.Context, raw, path string, size int64, wantHex string, progress Progress) error {
	resp, err := c.get(ctx, raw)
	if err != nil {
		return fmt.Errorf("couldn't download the update: %w", err)
	}
	defer resp.Body.Close()
	if resp.ContentLength >= 0 && resp.ContentLength != size {
		return errors.New("the download size doesn't match the release")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > size {
				return errors.New("the download is larger than the release says")
			}
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(done, size)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("the download was interrupted: %w", rerr)
		}
	}
	if done != size {
		return errors.New("the download ended early")
	}
	if progress != nil {
		progress(done, size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantHex {
		return errors.New("the download doesn't match its published SHA-256 checksum")
	}
	return f.Sync()
}

// sumFor finds name in a sha256sum-style list ("<hex>  <name>" or "<hex> *<name>").
func sumFor(list []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(list))
	found := ""
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		if strings.TrimPrefix(f[1], "*") == name {
			if found != "" {
				return "", errors.New("the checksum list names the download twice")
			}
			found = strings.ToLower(f[0])
		}
	}
	if !hexSHA256.MatchString(found) {
		return "", errors.New("the checksum list has no valid SHA-256 for the download")
	}
	return found, nil
}
