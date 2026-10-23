package patcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAndRemovePatchToHTML(t *testing.T) {
	sample := `<!doctype html><html><head><title>CPA</title></head><body><script>const ANTIGRAVITY_QUOTA_URLS=["https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary","https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary","https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"];</script></body></html>`
	ua := "antigravity/hub/2.24.0 (aidev_client; os_type=darwin; arch=arm64; cl=1000335682)"

	patched, changed := ApplyPatchToHTML(sample, ua)
	if !changed {
		t.Fatal("expected ApplyPatchToHTML to report changed=true")
	}
	if !IsPatchedHTML(patched) {
		t.Fatal("expected IsPatchedHTML to be true")
	}
	if !strings.Contains(patched, urlArrayVariants[1][1]) {
		t.Fatal("expected URL array order to prioritize cloudcode-pa.googleapis.com")
	}
	if !strings.Contains(patched, ua) {
		t.Fatal("expected patched HTML to contain updated Hub User-Agent")
	}

	// Applying again with the same UA should be idempotent.
	patchedAgain, changedAgain := ApplyPatchToHTML(patched, ua)
	if changedAgain || patchedAgain != patched {
		t.Fatal("expected second ApplyPatchToHTML with same UA to be idempotent")
	}

	// Restoring should remove the patch block and restore original URL order.
	restored, restoredChanged := RemovePatchFromHTML(patched)
	if !restoredChanged {
		t.Fatal("expected RemovePatchFromHTML to report changed=true")
	}
	if IsPatchedHTML(restored) {
		t.Fatal("expected IsPatchedHTML to be false after restore")
	}
	if !strings.Contains(restored, urlArrayVariants[1][0]) {
		t.Fatal("expected original URL array order after restore")
	}
}

func TestPatcherEnsurePatchedFile(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "management.html")
	orig := `<html><head></head><body>` + urlArrayVariants[0][0] + `</body></html>`
	if err := os.WriteFile(htmlPath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(true, []string{htmlPath}, func() string {
		return "antigravity/hub/2.25.0 (aidev_client; os_type=darwin; arch=arm64)"
	})
	st := p.EnsurePatched()
	if !st.Patched {
		t.Fatalf("expected Patched=true, got %+v", st)
	}

	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPatchedHTML(string(raw)) {
		t.Fatal("expected file on disk to be patched")
	}
}

func TestApplyPatchHandlesBacktickBundleAndKeepsCharsetFirst(t *testing.T) {
	// Shape of the current Vite/Rolldown management.html bundle.
	sample := "<!doctype html>\n<html>\n  <head>\n    <meta charset=\"UTF-8\" />\n    <script type=\"module\" crossorigin>vee=" +
		urlArrayVariants[0][0] + "</script>\n  </head><body></body></html>"

	patched, changed := ApplyPatchToHTML(sample, "")
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(patched, urlArrayVariants[0][1]) {
		t.Fatal("expected backtick URL array to be reordered")
	}
	charsetIdx := strings.Index(patched, `<meta charset="UTF-8" />`)
	scriptIdx := strings.Index(patched, PatchMarkerStart)
	moduleIdx := strings.Index(patched, `<script type="module"`)
	if !(charsetIdx >= 0 && charsetIdx < scriptIdx && scriptIdx < moduleIdx) {
		t.Fatalf("expected meta charset < patch < module bundle, got %d %d %d", charsetIdx, scriptIdx, moduleIdx)
	}
	if !strings.Contains(patched, "QUOTA_URLS") || !strings.Contains(patched, "XMLHttpRequest") {
		t.Fatal("expected XHR interceptor replaying api-call across quota endpoints")
	}

	restored, _ := RemovePatchFromHTML(patched)
	if restored != sample {
		t.Fatalf("expected exact restore, got:\n%s", restored)
	}
}

func TestEnsurePatchedWritesAtomicallyAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "management.html")
	if err := os.WriteFile(htmlPath, []byte(`<html><head></head><body></body></html>`), 0o640); err != nil {
		t.Fatal(err)
	}
	p := New(true, []string{htmlPath}, nil)
	if st := p.EnsurePatched(); !st.Patched {
		t.Fatalf("expected patched, got %+v", st)
	}
	info, err := os.Stat(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".management.html.agcp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
