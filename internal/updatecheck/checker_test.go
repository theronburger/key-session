package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckFindsAndCachesUpdate(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Header.Get("User-Agent") != "key-session/0.2.0" {
			t.Fatalf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.invalid/release"}`))
	}))
	defer server.Close()

	now := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	checker := Checker{
		APIURL:    server.URL,
		CachePath: filepath.Join(t.TempDir(), "cache", "update-check.json"),
		Client:    server.Client(),
		Now:       func() time.Time { return now },
	}
	result, err := checker.Check(context.Background(), "0.2.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "0.3.0" {
		t.Fatalf("result = %+v", result)
	}

	server.Close()
	result, err = checker.Check(context.Background(), "0.2.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || requestCount != 1 {
		t.Fatalf("cached result = %+v, request count = %d", result, requestCount)
	}
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "0.3.0", right: "0.2.9", want: 1},
		{left: "1.0.0", right: "1.0.0", want: 0},
		{left: "1.2.3", right: "2.0.0", want: -1},
		{left: "1.0.0", right: "1.0.0-rc.1", want: 1},
		{left: "1.0.0-rc.2", right: "1.0.0-rc.10", want: -1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
