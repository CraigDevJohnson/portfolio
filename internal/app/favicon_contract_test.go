package app

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const faviconAssetVersion = "20260819a"

func TestPublicShellPublishesCompleteEmberglassFaviconSet(t *testing.T) {
	application := newTestApp(t)
	mux, _ := buildMux(application, application.Logger, true)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	markers := []string{
		`<meta name="theme-color" content="#17121B">`,
		`<link rel="icon" type="image/svg+xml" href="/static/images/favicon.svg?v=` + faviconAssetVersion + `">`,
		`<link rel="icon" type="image/png" sizes="32x32" href="/static/images/favicon-32x32.png?v=` + faviconAssetVersion + `">`,
		`<link rel="icon" type="image/png" sizes="16x16" href="/static/images/favicon-16x16.png?v=` + faviconAssetVersion + `">`,
		`<link rel="icon" type="image/x-icon" href="/static/images/favicon.ico?v=` + faviconAssetVersion + `">`,
		`<link rel="apple-touch-icon" sizes="180x180" href="/static/images/apple-touch-icon.png?v=` + faviconAssetVersion + `">`,
		`<link rel="manifest" href="/static/images/site.webmanifest?v=` + faviconAssetVersion + `">`,
	}
	last := -1
	for _, marker := range markers {
		index := strings.Index(body, marker)
		if index < 0 {
			t.Errorf("GET / missing favicon marker %q", marker)
			continue
		}
		if index <= last {
			t.Errorf("favicon marker %q is out of order", marker)
		}
		last = index
	}
}

func TestFaviconAssetsUseRequiredSizesAndEmberglassPalette(t *testing.T) {
	requiredPNGs := map[string]image.Point{
		"favicon-16x16.png":          {X: 16, Y: 16},
		"favicon-32x32.png":          {X: 32, Y: 32},
		"apple-touch-icon.png":       {X: 180, Y: 180},
		"android-chrome-192x192.png": {X: 192, Y: 192},
		"android-chrome-512x512.png": {X: 512, Y: 512},
	}

	for name, want := range requiredPNGs {
		t.Run(name, func(t *testing.T) {
			file, err := os.Open(faviconPath(name))
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer file.Close()

			config, err := png.DecodeConfig(file)
			if err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
			if config.Width != want.X || config.Height != want.Y {
				t.Errorf("%s dimensions = %dx%d, want %dx%d", name, config.Width, config.Height, want.X, want.Y)
			}
		})
	}

	file, err := os.Open(faviconPath("apple-touch-icon.png"))
	if err != nil {
		t.Fatalf("open apple touch icon: %v", err)
	}
	defer file.Close()
	icon, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode apple touch icon: %v", err)
	}

	palette := map[string]color.RGBA{
		"night mulberry":   {R: 0x17, G: 0x12, B: 0x1b, A: 0xff},
		"campfire apricot": {R: 0xff, G: 0xa6, B: 0x77, A: 0xff},
		"rosehip":          {R: 0xff, G: 0x7f, B: 0xa8, A: 0xff},
		"pond mint":        {R: 0x78, G: 0xe3, B: 0xc3, A: 0xff},
	}
	minimumCoverage := map[string]float64{
		"night mulberry":   0.20,
		"campfire apricot": 0.20,
		"rosehip":          0.01,
		"pond mint":        0.01,
	}
	for name, target := range palette {
		if coverage := faviconColorCoverage(icon, target, 10); coverage < minimumCoverage[name] {
			t.Errorf("apple touch icon %s coverage = %.3f, want at least %.3f", name, coverage, minimumCoverage[name])
		}
	}
}

func TestFaviconManifestUsesEmberglassInstallColorsAndCurrentIcons(t *testing.T) {
	manifestBytes, err := os.ReadFile(faviconPath("site.webmanifest"))
	if err != nil {
		t.Fatalf("read favicon manifest: %v", err)
	}
	var manifest struct {
		ThemeColor      string `json:"theme_color"`
		BackgroundColor string `json:"background_color"`
		Icons           []struct {
			Source string `json:"src"`
			Sizes  string `json:"sizes"`
			Type   string `json:"type"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode favicon manifest: %v", err)
	}
	if manifest.ThemeColor != "#17121B" || manifest.BackgroundColor != "#17121B" {
		t.Errorf("manifest colors = theme:%q background:%q, want Emberglass night mulberry", manifest.ThemeColor, manifest.BackgroundColor)
	}
	wantIcons := map[string]string{
		"192x192": "/static/images/android-chrome-192x192.png?v=" + faviconAssetVersion,
		"512x512": "/static/images/android-chrome-512x512.png?v=" + faviconAssetVersion,
	}
	if len(manifest.Icons) != len(wantIcons) {
		t.Fatalf("manifest icon count = %d, want %d", len(manifest.Icons), len(wantIcons))
	}
	for _, icon := range manifest.Icons {
		if icon.Source != wantIcons[icon.Sizes] || icon.Type != "image/png" {
			t.Errorf("manifest icon %q = source:%q type:%q", icon.Sizes, icon.Source, icon.Type)
		}
	}
}

func faviconPath(name string) string {
	return filepath.Join("..", "..", "cmd", "web", "static", "images", name)
}

func faviconColorCoverage(icon image.Image, target color.RGBA, tolerance uint32) float64 {
	bounds := icon.Bounds()
	matched := 0
	total := bounds.Dx() * bounds.Dy()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, alpha := icon.At(x, y).RGBA()
			if alpha < 0x8000 {
				continue
			}
			if faviconChannelNear(red>>8, uint32(target.R), tolerance) &&
				faviconChannelNear(green>>8, uint32(target.G), tolerance) &&
				faviconChannelNear(blue>>8, uint32(target.B), tolerance) {
				matched++
			}
		}
	}
	return float64(matched) / float64(total)
}

func faviconChannelNear(got, want, tolerance uint32) bool {
	if got > want {
		return got-want <= tolerance
	}
	return want-got <= tolerance
}
