package plugin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
)

func TestPluginRegisterAndManagementRoutes(t *testing.T) {
	dir := t.TempDir()
	staticDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(staticDir, 0o755)
	htmlPath := filepath.Join(staticDir, "management.html")
	_ = os.WriteFile(htmlPath, []byte(`<html><head></head><body></body></html>`), 0o644)

	p := New(nil, "0.1.0", dir, filepath.Join(dir, "config.yaml"))
	defer p.Shutdown()

	regRaw := p.Handle(abi.MethodPluginRegister, []byte(`{"schema_version":6,"config_yaml":""}`))
	var env struct {
		OK     bool             `json:"ok"`
		Result abi.Registration `json:"result"`
	}
	if err := json.Unmarshal(regRaw, &env); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	if !env.OK {
		t.Fatalf("register failed: %s", string(regRaw))
	}
	if !env.Result.Capabilities["quota_provider"] || !env.Result.Capabilities["management_api"] {
		t.Fatalf("unexpected capabilities: %+v", env.Result.Capabilities)
	}

	// Verify management.html was automatically patched on registration.
	st := p.patcher.Status()
	if !st.Patched {
		t.Fatalf("expected management.html to be auto-patched on register, got %+v", st)
	}

	// Verify quota.identifier and quota.describe.
	idRaw := p.Handle(abi.MethodQuotaIdentifier, nil)
	if string(idRaw) == "" {
		t.Fatal("empty quota.identifier response")
	}

	// Verify management.handle for resource page.
	reqBytes, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/antigravity-category-plugin/page",
	})
	pageEnvRaw := p.Handle(abi.MethodManagementHandle, reqBytes)
	var pageEnv struct {
		OK     bool                   `json:"ok"`
		Result abi.ManagementResponse `json:"result"`
	}
	if err := json.Unmarshal(pageEnvRaw, &pageEnv); err != nil {
		t.Fatal(err)
	}
	if !pageEnv.OK || pageEnv.Result.StatusCode != http.StatusOK || len(pageEnv.Result.Body) == 0 {
		t.Fatalf("unexpected page response: %+v", pageEnv)
	}

	// A library file named with a suffix yields a different plugin ID; the page must still be served.
	altReq, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/antigravity-category-plugin_linux_amd64/page",
	})
	if err := json.Unmarshal(p.Handle(abi.MethodManagementHandle, altReq), &pageEnv); err != nil {
		t.Fatal(err)
	}
	if ct := pageEnv.Result.Headers.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected HTML page for alternate plugin ID, got %q", ct)
	}

	// quota-summary without a usable credential must fail with non-2xx so the panel falls back.
	qsReq, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/antigravity-category-plugin/quota-summary",
		Body:   []byte(`{"authIndex":"","data":"{\"project\":\"p\"}"}`),
	})
	if err := json.Unmarshal(p.Handle(abi.MethodManagementHandle, qsReq), &pageEnv); err != nil {
		t.Fatal(err)
	}
	if pageEnv.Result.StatusCode < 400 {
		t.Fatalf("expected error status for quota-summary without auth_index, got %d", pageEnv.Result.StatusCode)
	}
}
