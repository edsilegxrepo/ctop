package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckUpdateAndFindAsset(t *testing.T) {
	mockRel := githubRelease{
		TagName: "v0.9.5",
		Name:    "v0.9.5 Release",
		Assets: []githubAsset{
			{Name: "ctop-0.9.5-linux-amd64", BrowserDownloadURL: "https://example.com/ctop-linux-amd64"},
			{Name: "ctop-0.9.5-windows-amd64.exe", BrowserDownloadURL: "https://example.com/ctop-windows-amd64.exe"},
			{Name: "ctop-0.9.5-darwin-arm64", BrowserDownloadURL: "https://example.com/ctop-darwin-arm64"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockRel)
	}))
	defer server.Close()

	// Parse custom URL via CheckUpdate with test mock
	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to query mock server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		t.Fatalf("failed to decode release: %v", err)
	}

	if rel.TagName != "v0.9.5" {
		t.Errorf("expected tag v0.9.5, got %s", rel.TagName)
	}

	// Test asset matching for Linux
	linuxAsset, err := FindAsset(&rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("failed to find linux asset: %v", err)
	}
	if !strings.Contains(linuxAsset.Name, "linux-amd64") {
		t.Errorf("expected linux asset, got %s", linuxAsset.Name)
	}

	// Test asset matching for Windows
	winAsset, err := FindAsset(&rel, "windows", "amd64")
	if err != nil {
		t.Fatalf("failed to find windows asset: %v", err)
	}
	if !strings.Contains(winAsset.Name, "windows-amd64.exe") {
		t.Errorf("expected windows asset, got %s", winAsset.Name)
	}
}
