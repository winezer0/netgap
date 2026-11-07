//go:build acceptance

package updater

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLiveGitHubReleaseDownloadAndExtract(t *testing.T) {
	if os.Getenv("NETGAP_LIVE_UPDATE_ACCEPTANCE") != "1" {
		t.Skip("set NETGAP_LIVE_UPDATE_ACCEPTANCE=1 to download the current GitHub Release")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	root := t.TempDir()
	apiClient := &http.Client{Timeout: 30 * time.Second}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		apiClient.Transport = githubTokenRoundTripper{token: token}
	}
	manager := &Manager{
		root:        root,
		pendingPath: filepath.Join(root, "pending.json"),
		apiClient:   apiClient,
		dlClient:    &http.Client{Timeout: 10 * time.Minute},
	}
	check, err := manager.Check(ctx, "v0.0.0")
	if err != nil {
		t.Fatalf("check latest stable GitHub Release: %v", err)
	}
	if !check.Available {
		t.Fatalf("latest stable Release %q is not newer than the acceptance baseline", check.LatestVersion)
	}
	if _, err := manager.Download(ctx); err != nil {
		t.Fatalf("download, verify, and extract GitHub Release: %v", err)
	}
	if manager.staged == nil {
		t.Fatal("verified Release payload was not staged")
	}
	stageWithin := withinRoot(root, manager.staged.StageDir)
	sourceWithin := withinRoot(root, manager.staged.SourcePath)
	if !stageWithin || !sourceWithin {
		rootAbs, _ := filepath.Abs(root)
		stageAbs, _ := filepath.Abs(manager.staged.StageDir)
		sourceAbs, _ := filepath.Abs(manager.staged.SourcePath)
		stageRel, stageErr := filepath.Rel(rootAbs, stageAbs)
		sourceRel, sourceErr := filepath.Rel(rootAbs, sourceAbs)
		t.Fatalf("staged paths escaped temporary acceptance root: root=%q stage=%q (rel=%q err=%v within=%t) source=%q (rel=%q err=%v within=%t)", rootAbs, stageAbs, stageRel, stageErr, stageWithin, sourceAbs, sourceRel, sourceErr, sourceWithin)
	}
	info, err := os.Stat(manager.staged.SourcePath)
	if err != nil {
		t.Fatalf("inspect extracted Release payload: %v", err)
	}
	payloadPattern, err := payloadPatternForPlatform()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(manager.staged.SourcePath) != payloadPattern {
		t.Fatalf("resolved payload = %q, want platform payload %q", filepath.Base(manager.staged.SourcePath), payloadPattern)
	}
	wantDirectory := runtime.GOOS == "darwin"
	if wantDirectory != info.IsDir() {
		t.Fatalf("payload directory status = %t, want %t", info.IsDir(), wantDirectory)
	}
	t.Logf("downloaded and verified %s (%s); payload=%s", check.LatestVersion, manager.staged.AssetName, manager.staged.SourcePath)
	// t.TempDir removes the downloaded archive, extracted payload, and metadata.
	// Install is intentionally not called: this acceptance test never replaces the running application.
}

type githubTokenRoundTripper struct {
	token string
}

func (t githubTokenRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	authenticated := request.Clone(request.Context())
	authenticated.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(authenticated)
}
