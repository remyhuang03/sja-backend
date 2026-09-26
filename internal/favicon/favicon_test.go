package favicon

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"net/netip"
	"net/url"
	"testing"
)

func TestPublicNetworkOnly(t *testing.T) {
	for _, raw := range []string{"http://localhost/", "http://127.0.0.1/", "http://169.254.169.254/", "https://10.0.0.1/", "http://[::1]/", "https://user:pass@example.org/", "file:///etc/passwd", "http://example.org:8080/"} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Error("accepted", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "198.18.0.1", "::ffff:127.0.0.1", "fe80::1", "fc00::1", "2001:db8::1"} {
		if publicIP(netip.MustParseAddr(raw)) {
			t.Error("accepted IP", raw)
		}
	}
	if !publicIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public IP rejected")
	}
	if got, err := NormalizeURL("https://EXAMPLE.org#fragment"); err != nil || got != "https://example.org/" {
		t.Fatal(got, err)
	}
	if _, err := safeClient().Get("http://127.0.0.1/"); err == nil {
		t.Fatal("loopback dial must fail")
	}
	if got := FetchPNG(context.Background(), "http://169.254.169.254/"); len(got) != 0 {
		t.Fatal("metadata endpoint fetched")
	}
}
func TestMetadataAndNormalization(t *testing.T) {
	page, _ := url.Parse("https://example.org/app/")
	got := candidates([]byte(`<base href="/assets/"><link rel="shortcut ICON" href="logo.png?a=1&amp;b=2"><link rel="icon" href="http://127.0.0.1/x"><link rel="apple-touch-icon" href="//cdn.example.org/icon.png">`), page)
	if len(got) != 3 || got[0] != "https://example.org/assets/logo.png?a=1&b=2" || got[2] != "https://example.org/favicon.ico" {
		t.Fatal(got)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(2, 2, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	pngBytes := b.Bytes()
	ico := make([]byte, 22)
	copy(ico, []byte{0, 0, 1, 0, 1, 0})
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(pngBytes)))
	binary.LittleEndian.PutUint32(ico[18:22], 22)
	ico = append(ico, pngBytes...)
	for _, data := range [][]byte{pngBytes, ico} {
		out, err := normalizeImage(data)
		if err != nil {
			t.Fatal(err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
		if err != nil || format != "png" || cfg.Width != 32 || cfg.Height != 32 {
			t.Fatal(cfg, format, err)
		}
	}
	for _, data := range [][]byte{[]byte(`<svg onload="alert(1)"></svg>`), []byte(`<html>not an image</html>`)} {
		if _, err := normalizeImage(data); err == nil {
			t.Fatal("active content accepted")
		}
	}
}
