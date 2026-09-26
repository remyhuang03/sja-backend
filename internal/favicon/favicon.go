// Package favicon discovers and caches website icons without accessing private networks.
package favicon

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/net/html"
)

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(cidr).Contains(ip) {
			return false
		}
	}
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}

func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || len(raw) > 2048 {
		return "", errors.New("invalid website URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return "", errors.New("non-public host")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicIP(ip) {
		return "", errors.New("non-public IP")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return "", errors.New("unsupported port")
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func safeClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, errors.New("no public address")
			}
			for _, ip := range ips {
				if !publicIP(ip) {
					return nil, errors.New("blocked address")
				}
			}
			// Dial the checked address directly to prevent DNS rebinding.
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		}, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		_, err := NormalizeURL(req.URL.String())
		return err
	}}
}
func download(ctx context.Context, c *http.Client, raw string, limit int64) ([]byte, *url.URL, error) {
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", normalized, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "SJA-IconFetcher/1.0 (+https://sja.remya.top)")
	res, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, nil, errors.New("icon request failed")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, nil, errors.New("icon response too large")
	}
	return data, res.Request.URL, nil
}
func candidates(data []byte, page *url.URL) []string {
	root := page.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return []string{root}
	}
	base := page
	foundBase := false
	var links []string
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		attrs := map[string]string{}
		for _, a := range n.Attr {
			attrs[a.Key] = a.Val
		}
		if n.Data == "base" && !foundBase && attrs["href"] != "" {
			if u, e := page.Parse(attrs["href"]); e == nil {
				base = u
				foundBase = true
			}
		}
		if n.Data == "link" && attrs["href"] != "" {
			for _, rel := range strings.Fields(strings.ToLower(attrs["rel"])) {
				if rel == "icon" || rel == "apple-touch-icon" {
					links = append(links, attrs["href"])
					break
				}
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, href := range links {
		u, e := base.Parse(href)
		if e != nil {
			continue
		}
		raw, e := NormalizeURL(u.String())
		if e == nil && !seen[raw] {
			out = append(out, raw)
			seen[raw] = true
		}
		if len(out) == 4 {
			break
		}
	}
	return append(out, root)
}
func normalizeImage(data []byte) ([]byte, error) {
	// ICO containers may include PNG frames. Unsupported bitmap frames are skipped.
	if len(data) >= 6 && bytes.Equal(data[:4], []byte{0, 0, 1, 0}) {
		count := int(binary.LittleEndian.Uint16(data[4:6]))
		for i := 0; i < count && 6+(i+1)*16 <= len(data); i++ {
			entry := data[6+i*16 : 6+(i+1)*16]
			size := uint64(binary.LittleEndian.Uint32(entry[8:12]))
			offset := uint64(binary.LittleEndian.Uint32(entry[12:16]))
			if size >= 8 && offset+size <= uint64(len(data)) && bytes.Equal(data[offset:offset+8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
				data = data[offset : offset+size]
				break
			}
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 2048 || cfg.Height > 2048 {
		return nil, errors.New("icon dimensions too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	err = png.Encode(&out, dst)
	return out.Bytes(), err
}

// FetchPNG is best-effort. Missing or unsupported icons do not block publication.
func FetchPNG(ctx context.Context, raw string) []byte {
	raw, err := NormalizeURL(raw)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := safeClient()
	defer c.CloseIdleConnections()
	page, _ := url.Parse(raw)
	urls := []string{page.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()}
	if data, final, err := download(ctx, c, raw, 512<<10); err == nil {
		urls = candidates(data, final)
	}
	for _, candidate := range urls {
		data, _, err := download(ctx, c, candidate, 2<<20)
		if err == nil {
			if png, err := normalizeImage(data); err == nil {
				return png
			}
		}
	}
	return nil
}
