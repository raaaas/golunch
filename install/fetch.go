package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raaaas/golunch/instance"
	"github.com/raaaas/golunch/proxy"
)

// defaultMaxScriptBytes bounds how much of an installer is downloaded. Vendor
// install scripts are hand-written shell of a few tens of kilobytes, so 1 MiB is
// already generous; the alternative is an unbounded download of whatever a
// compromised endpoint decides to serve on the way to being executed.
const defaultMaxScriptBytes int64 = 1 << 20

// fetchTimeout bounds the transfer when the caller's context does not. A stalled
// CDN must not be able to pin a golunch process indefinitely.
const fetchTimeout = 2 * time.Minute

// userAgent names this tool on the wire. Some vendors' CDNs refuse the stock
// Go client; claiming to be curl would be a lie that makes debugging harder.
const userAgent = "golunch-install (https://github.com/raaaas/golunch)"

// Fetch downloads an installer script to dst and returns the hex sha256 of the
// bytes it wrote.
//
// It is exported separately from Install because consent has to be possible: the
// CLI stream fetches, prints the hash and the script's own notes, asks, and only
// then calls Install with the saved path. Nothing here executes anything.
//
// The proxy used is the instance's resolved decision, not the one golunch itself
// happens to be running behind; see proxyFunc.
func Fetch(ctx context.Context, scriptURL, dst string, maxBytes int64, res proxy.Resolution) (string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxScriptBytes
	}
	u, err := parseScriptURL(scriptURL)
	if err != nil {
		return "", err
	}
	proxyFn, err := proxyFunc(res)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout:   fetchTimeout,
		Transport: &http.Transport{Proxy: proxyFn},
		// A redirect is the one place the bytes that get executed come from
		// somewhere the user did not type, so every hop has to pass the same
		// scheme gate: an https installer that 302s to plaintext http is not the
		// transaction that was approved.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			_, err := parseScriptURL(req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("install: bad script URL %q: %w", scriptURL, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("install: fetch %s: %w", scriptURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("install: fetch %s: server answered %s", scriptURL, resp.Status)
	}

	// One byte past the limit is what distinguishes "exactly at the limit" from
	// "over it", which io.LimitReader alone cannot tell.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("install: read %s: %w", scriptURL, err)
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("install: script at %s is larger than the %s download limit; "+
			"a vendor installer is tens of kilobytes, so raise the limit only if you have read it",
			scriptURL, instance.FormatBytes(uint64(maxBytes)))
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("install: create %s: %w", filepath.Dir(dst), err)
	}
	// 0644 on purpose: this is content, not a command. It is run by handing the
	// path to an interpreter, and the audit copy in the logs directory is the
	// artifact worth keeping.
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return "", fmt.Errorf("install: write %s: %w", dst, err)
	}
	return digest, nil
}

// parseScriptURL is the scheme gate. The fetched bytes are executed, so
// plaintext http is refused outright: it is trivially tampered with in transit,
// and every vendor worth installing serves https. The single exception is a
// loopback URL, whose traffic never leaves the machine — this package's offline
// test suite runs against httptest's plaintext server, and serving an installer
// from a local process is a legitimate thing to do with an explicit URL.
//
// file: is not translated into a local read. Spec.ScriptPath is how a script
// already on disk is supplied, and keeping the two channels separate means the
// absence of a URL in metadata always means nothing was downloaded.
func parseScriptURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("install: no script URL given")
	}
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		return nil, fmt.Errorf("install: %q is a local file; pass it as Spec.ScriptPath rather than a URL", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("install: bad script URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("install: script URL %q has no host", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u, nil
	case "http":
		if isLoopback(u.Hostname()) {
			return u, nil
		}
		return nil, fmt.Errorf("install: refusing to download an executable script over plaintext http from %q; use https, or hand the file over with Spec.ScriptPath", u.Host)
	default:
		return nil, fmt.Errorf("install: unsupported script URL scheme %q in %q", u.Scheme, raw)
	}
}

func isLoopback(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// proxyFunc builds the transport's proxy selector for one instance's decided
// proxy, and is deliberately not http.ProxyFromEnvironment.
//
// The ambient environment is golunch's own, and that is a different question
// from the instance's: an instance configured with proxy = "none" must not have
// its installer downloaded through the corporate proxy because the person
// running golunch happens to be behind one, and an instance with a proxy
// configured must use it even when golunch was started with no proxy variables
// at all. BuildEnv already wrote the same decision into Spec.Env for the
// script's own curl (instance/env.go applyProxy); this is that decision for the
// Go side, and reading the environment here would let the two disagree.
func proxyFunc(res proxy.Resolution) (func(*http.Request) (*url.URL, error), error) {
	if res.Profile.Disabled() {
		return func(*http.Request) (*url.URL, error) { return nil, nil }, nil
	}
	// Parsed once, up front: proxy URLs have already been through
	// proxy.FromSpec, so a malformed one here is a config the caller built by
	// hand, and it should fail as a configuration error rather than as an
	// unexplained transport failure on the first hop.
	var httpsProxy, httpProxy, socksProxy *url.URL
	for _, f := range []struct {
		dst   **url.URL
		raw   string
		label string
	}{
		{&httpsProxy, res.Profile.HTTPS, "https"},
		{&httpProxy, res.Profile.HTTP, "http"},
		{&socksProxy, res.Profile.SOCKS, "socks"},
	} {
		if f.raw == "" {
			continue
		}
		u, err := url.Parse(f.raw)
		if err != nil {
			return nil, fmt.Errorf("install: %s proxy %q is not usable: %w", f.label, f.raw, err)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("install: %s proxy %q has no host", f.label, f.raw)
		}
		*f.dst = u
	}
	// The exception list is the merged one BuildEnv renders into NO_PROXY, so
	// the fetch and the script's curl bypass the same hosts.
	except := proxy.MergeEntries(res.NoProxyExtra, res.Profile.NoProxy, proxy.Floor())

	return func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		if host == "" {
			return nil, nil
		}
		if exceptHost(except, host) {
			return nil, nil
		}
		secure := strings.EqualFold(req.URL.Scheme, "https")
		if secure {
			switch {
			case httpsProxy != nil:
				return httpsProxy, nil
			case socksProxy != nil:
				return socksProxy, nil
			}
			// HTTPS unset while HTTP is set is a real configuration; the http
			// upstream tunnels this one with CONNECT, which is exactly the
			// fallback applyProxy applies to the child's HTTPS_PROXY.
			return httpProxy, nil
		}
		switch {
		case httpProxy != nil:
			return httpProxy, nil
		case socksProxy != nil:
			return socksProxy, nil
		case httpsProxy != nil:
			return httpsProxy, nil
		}
		// Nothing was configured anywhere: direct, which is not the same as the
		// "none" sentinel's deliberate unset.
		return nil, nil
	}, nil
}

// exceptHost reports whether a request host is exempt from proxying.
//
// The entry list mixes two matching languages and this function has to speak
// both. proxy.Floor() ships the literal "127.0.0.1" *and* the range
// "127.0.0.0/8" (see the comment at proxy/proxy.go:82-86) because Go's
// httpproxy and Node's proxy-agent honour CIDRs while curl — which is what every
// vendor installer shells out to — matches entries as plain suffixes and would
// route a request to http://127.0.0.1:4096 out through the proxy if only the
// range were present. Matching the union of both forms is what keeps the Go-side
// fetch and the script's own curl in agreement about one NO_PROXY.
//
// curl's `*` wildcards are deliberately unsupported: nothing in this repo's
// config path or registry produces them, and half-implementing a wildcard
// language is how an exception quietly stops matching anything.
func exceptHost(entries []string, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	ip := net.ParseIP(host)
	for _, raw := range entries {
		e := strings.ToLower(strings.TrimSpace(raw))
		if e == "" {
			continue
		}
		if _, netRange, err := net.ParseCIDR(e); err == nil && ip != nil && netRange.Contains(ip) {
			return true
		}
		// ".example.com" and "example.com" name the same zone: a leading dot is
		// how the suffix form is written, and the suffix form is how curl writes
		// it, so accepting both keeps a hand-edited config from silently
		// narrowing to one host.
		e = strings.TrimPrefix(e, ".")
		if host == e || strings.HasSuffix(host, "."+e) {
			return true
		}
		// "example.com:8080" is a host as far as this list is concerned. A
		// port-specific entry that failed to match would send internal traffic
		// back out through the proxy, which is the opposite of why it is here.
		// The base != "" guard is what keeps an IPv6 literal like "::1" from
		// being cut into an empty name that matches everything.
		if base, port, ok := strings.Cut(e, ":"); ok && base != "" && isDigits(port) {
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
