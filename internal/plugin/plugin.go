// Package plugin implements the CLIProxyAPI native plugin RPC methods.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
	"github.com/Azurefly/antigravity-category-plugin/internal/antigravity"
	"github.com/Azurefly/antigravity-category-plugin/internal/config"
	"github.com/Azurefly/antigravity-category-plugin/internal/host"
	"github.com/Azurefly/antigravity-category-plugin/internal/patcher"
	"github.com/Azurefly/antigravity-category-plugin/internal/web"
)

const (
	RepositoryURL = "https://github.com/Azurefly/antigravity-category-plugin"
	APIBase       = "/v0/management/" + config.PluginID
	PagePath      = "/page"
)

// Plugin coordinates Antigravity multi-endpoint quota fetching, management routes,
// and automatic hot-patching of management.html.
type Plugin struct {
	version        string
	host           host.Host
	workDir        string
	hostConfigPath string

	svc     *antigravity.Service
	patcher *patcher.Patcher

	mu           sync.RWMutex
	cfg          config.Config
	accounts     []antigravity.AccountQuotaResult
	refreshedAt  string
	refreshError string

	startOnce sync.Once
	stopCh    chan struct{}
	stopOnce  sync.Once
}

// New creates a new Plugin instance.
func New(h host.Host, version, workDir, hostConfigPath string) *Plugin {
	cfg := config.Default()
	svc := antigravity.NewService(h, cfg)
	candidates := config.ManagementHTMLCandidates(cfg.ManagementHTMLPath, hostConfigPath, workDir)
	pt := patcher.New(cfg.AutoPatchPanel, candidates, svc.CachedUserAgent)

	return &Plugin{
		version:        version,
		host:           h,
		workDir:        workDir,
		hostConfigPath: hostConfigPath,
		svc:            svc,
		patcher:        pt,
		cfg:            cfg,
		stopCh:         make(chan struct{}),
	}
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

// ErrorEnvelope encodes a failed RPC response.
func ErrorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{
		OK: false,
		Error: &envelopeError{
			Code:    code,
			Message: message,
		},
	})
	return raw
}

// Handle dispatches a single C-ABI RPC call from CLIProxyAPI.
func (p *Plugin) Handle(method string, request []byte) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			out = ErrorEnvelope("plugin_panic", fmt.Sprint(r))
		}
	}()
	res, err := p.handle(method, request)
	if err != nil {
		return ErrorEnvelope("plugin_error", err.Error())
	}
	return res
}

// Shutdown stops the background watcher goroutine.
func (p *Plugin) Shutdown() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
}

func (p *Plugin) handle(method string, request []byte) ([]byte, error) {
	switch method {
	case abi.MethodPluginRegister, abi.MethodPluginReconfigure:
		var req abi.LifecycleRequest
		if len(request) > 0 {
			_ = json.Unmarshal(request, &req)
		}
		cfg, _ := config.Parse(req.ConfigYAML)
		p.applyConfig(cfg)
		p.startOnce.Do(p.startBackgroundLoop)
		return okEnvelope(p.registration(req.SchemaVersion))

	case abi.MethodPluginQuiesce, abi.MethodPluginShutdown:
		p.Shutdown()
		return okEnvelope(struct{}{})

	case abi.MethodQuotaIdentifier:
		return okEnvelope(abi.IdentifierResponse{Identifier: "antigravity"})

	case abi.MethodQuotaDescribe:
		return okEnvelope(abi.QuotaDescribeResponse{
			SupportedProviders: []string{"antigravity"},
			DisplayName:        "Antigravity Category Quota (Gemini + Claude/GPT)",
			SupportsReset:      false,
		})

	case abi.MethodQuotaFetch:
		var req abi.QuotaFetchRequest
		if len(request) > 0 {
			if err := json.Unmarshal(request, &req); err != nil {
				return nil, fmt.Errorf("decode quota.fetch request: %w", err)
			}
		}
		ctx := host.WithCallbackID(context.Background(), req.HostCallbackID)
		resp, err := p.svc.FetchQuotaForAuth(ctx, req)
		if err != nil {
			return nil, err
		}
		return okEnvelope(resp)

	case abi.MethodQuotaReset:
		return okEnvelope(abi.QuotaResetResponse{
			Success: false,
			Message: "Antigravity quota windows reset automatically on schedule",
		})

	case abi.MethodManagementRegister:
		return okEnvelope(abi.ManagementRegistrationResponse{
			Routes: []abi.ManagementRoute{
				{
					Method:      http.MethodGet,
					Path:        APIBase + "/status",
					Description: "Get Antigravity quota status and management.html patch state",
				},
				{
					Method:      http.MethodPost,
					Path:        APIBase + "/refresh",
					Description: "Refresh Antigravity Gemini & Claude/GPT quota across all endpoints",
				},
				{
					Method:      http.MethodPost,
					Path:        APIBase + "/quota-summary",
					Description: "Management Center api-call compatible retrieveUserQuotaSummary with Gemini + Claude/GPT groups merged across endpoints",
				},
				{
					Method:      http.MethodPost,
					Path:        APIBase + "/patch",
					Description: "Apply Antigravity Gemini quota hot-patch to management.html",
				},
				{
					Method:      http.MethodPost,
					Path:        APIBase + "/unpatch",
					Description: "Restore management.html to unpatched state",
				},
			},
			Resources: []abi.ResourceRoute{
				{
					Path:        PagePath,
					Menu:        "Antigravity Quota",
					Description: "Antigravity Gemini & Claude/GPT 双算力池额度看板与原生面板修复",
				},
			},
		})

	case abi.MethodManagementHandle:
		var req abi.ManagementRequest
		if len(request) > 0 {
			if err := json.Unmarshal(request, &req); err != nil {
				return nil, fmt.Errorf("decode management.handle request: %w", err)
			}
		}
		return okEnvelope(p.handleManagement(req))
	}

	return ErrorEnvelope("unknown_method", "unknown method: "+method), nil
}

func (p *Plugin) applyConfig(cfg config.Config) {
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()

	p.svc.SetConfig(cfg)
	candidates := config.ManagementHTMLCandidates(cfg.ManagementHTMLPath, p.hostConfigPath, p.workDir)
	p.patcher.Configure(cfg.AutoPatchPanel, candidates)
	if cfg.AutoPatchPanel {
		p.patcher.EnsurePatched()
	}
}

func (p *Plugin) startBackgroundLoop() {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.mu.RLock()
				autoPatch := p.cfg.AutoPatchPanel
				p.mu.RUnlock()
				if autoPatch {
					p.patcher.EnsurePatched()
				}
			}
		}
	}()
}

func (p *Plugin) registration(hostSchemaVersion uint32) abi.Registration {
	schemaVer := uint32(abi.SchemaVersion)
	if hostSchemaVersion > 0 && hostSchemaVersion < schemaVer {
		schemaVer = hostSchemaVersion
	}
	return abi.Registration{
		SchemaVersion: schemaVer,
		Metadata: abi.Metadata{
			Name:             "Antigravity Category Quota",
			Version:          p.version,
			Author:           "Azurefly",
			GitHubRepository: RepositoryURL,
			ConfigFields: []abi.ConfigField{
				{
					Name:        "auto_patch_panel",
					Type:        abi.ConfigFieldTypeBoolean,
					Default:     true,
					Description: "自动热修补 static/management.html，使原生面板恢复显示 Antigravity Gemini 额度分组 / Automatically hot-patch static/management.html so built-in UI shows Gemini quota",
				},
				{
					Name:        "management_html_path",
					Type:        abi.ConfigFieldTypeString,
					Description: "自定义 management.html 路径（留空则自动探测 static/management.html） / Custom path to management.html (empty auto-detects)",
				},
				{
					Name:        "prefer_prod_endpoint",
					Type:        abi.ConfigFieldTypeBoolean,
					Default:     true,
					Description: "优先请求正式地址 cloudcode-pa.googleapis.com 获取包含 Gemini 的完整分组 / Query cloudcode-pa.googleapis.com first",
				},
				{
					Name:        "merge_endpoint_groups",
					Type:        abi.ConfigFieldTypeBoolean,
					Default:     true,
					Description: "当单端点缺失 Gemini 或 Claude/GPT 分组时，自动请求其余端点并合并额度分组 / Merge quota groups across endpoints",
				},
				{
					Name:        "user_agent",
					Type:        abi.ConfigFieldTypeString,
					Description: "自定义 Antigravity User-Agent（留空则自动同步最新 Antigravity Hub 版本指纹） / Custom Antigravity User-Agent",
				},
			},
		},
		Capabilities: map[string]bool{
			"management_api": true,
			"quota_provider": true,
		},
	}
}

func (p *Plugin) handleManagement(req abi.ManagementRequest) abi.ManagementResponse {
	path := strings.TrimRight(req.Path, "/")
	// Match any plugin ID segment: CPA derives the ID from the library file name.
	if strings.HasPrefix(path, "/v0/resource/plugins/") && strings.HasSuffix(path, PagePath) {
		return abi.ManagementResponse{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Content-Type":  []string{"text/html; charset=utf-8"},
				"Cache-Control": []string{"no-store"},
			},
			Body: web.Page(),
		}
	}

	ctx := host.WithCallbackID(context.Background(), req.HostCallbackID)
	var body struct {
		AuthIndex string `json:"auth_index"`
	}
	if len(req.Body) > 0 {
		_ = json.Unmarshal(req.Body, &body)
	}

	switch strings.ToUpper(req.Method) + " " + path {
	case "GET " + APIBase + "/status":
		return jsonResponse(http.StatusOK, p.statusPayload())

	case "POST " + APIBase + "/refresh":
		p.refreshAccounts(ctx, strings.TrimSpace(body.AuthIndex))
		return jsonResponse(http.StatusOK, p.statusPayload())

	case "POST " + APIBase + "/quota-summary":
		return p.handleQuotaSummary(ctx, req.Body)

	case "POST " + APIBase + "/patch":
		p.patcher.EnsurePatched()
		return jsonResponse(http.StatusOK, p.statusPayload())

	case "POST " + APIBase + "/unpatch":
		p.patcher.Restore()
		return jsonResponse(http.StatusOK, p.statusPayload())
	}

	return jsonResponse(http.StatusNotFound, map[string]string{
		"error": fmt.Sprintf("unknown route %s %s", req.Method, req.Path),
	})
}

// quotaSummaryTimeout bounds the whole rerouted api-call (UA refresh, token refresh, every
// endpoint). It stays below the injected script's client-side timeout so the panel gets
// an answer, or falls back, instead of loading forever.
const quotaSummaryTimeout = 25 * time.Second

// handleQuotaSummary serves the rerouted Management Center api-call for
// retrieveUserQuotaSummary. Non-2xx tells the injected script to fall back to api-call.
func (p *Plugin) handleQuotaSummary(ctx context.Context, raw []byte) abi.ManagementResponse {
	ctx, cancel := context.WithTimeout(ctx, quotaSummaryTimeout)
	defer cancel()
	var req struct {
		AuthIndexSnake string `json:"auth_index"`
		AuthIndexCamel string `json:"authIndex"`
		Data           string `json:"data"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	var data struct {
		Project string `json:"project"`
	}
	_ = json.Unmarshal([]byte(req.Data), &data)

	authIndex := strings.TrimSpace(req.AuthIndexCamel)
	if authIndex == "" {
		authIndex = strings.TrimSpace(req.AuthIndexSnake)
	}
	resp, err := p.svc.FetchQuotaSummary(ctx, authIndex, data.Project)
	if err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	return jsonResponse(http.StatusOK, resp)
}

// refreshConcurrency bounds parallel upstream quota lookups across credentials.
const refreshConcurrency = 4

func (p *Plugin) refreshAccounts(ctx context.Context, targetAuthIndex string) {
	if p.host == nil {
		p.setRefreshResult(nil, "host unavailable")
		return
	}
	files, err := p.host.AuthList()
	if err != nil {
		p.setRefreshResult(nil, "list credentials failed: "+err.Error())
		return
	}
	var targets []abi.HostAuthFileEntry
	for _, f := range files {
		prov := strings.ToLower(strings.TrimSpace(f.Provider))
		if prov == "" {
			prov = strings.ToLower(strings.TrimSpace(f.Type))
		}
		if prov != "antigravity" || f.Disabled {
			continue
		}
		if targetAuthIndex != "" && f.AuthIndex != targetAuthIndex {
			continue
		}
		targets = append(targets, f)
	}

	results := make([]antigravity.AccountQuotaResult, len(targets))
	sem := make(chan struct{}, refreshConcurrency)
	var wg sync.WaitGroup
	for i, f := range targets {
		wg.Add(1)
		go func(i int, f abi.HostAuthFileEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			detail, _ := p.svc.FetchAccountDetail(ctx, f.AuthIndex, nil, nil, nil)
			if detail.Email == "" {
				detail.Email = f.Email
			}
			if detail.Name == "" {
				detail.Name = f.Name
			}
			if detail.ProjectID == "" {
				detail.ProjectID = f.ProjectID
			}
			detail.RawPayload = nil
			results[i] = detail
		}(i, f)
	}
	wg.Wait()

	p.setRefreshResult(results, "")
}

func (p *Plugin) setRefreshResult(results []antigravity.AccountQuotaResult, refreshErr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if refreshErr == "" {
		p.accounts = results
	}
	p.refreshError = refreshErr
	p.refreshedAt = time.Now().UTC().Format(time.RFC3339)
}

func (p *Plugin) statusPayload() map[string]any {
	p.mu.RLock()
	accs := append([]antigravity.AccountQuotaResult(nil), p.accounts...)
	cfg := p.cfg
	refreshedAt, refreshErr := p.refreshedAt, p.refreshError
	p.mu.RUnlock()

	return map[string]any{
		"plugin":        config.PluginID,
		"version":       p.version,
		"user_agent":    p.svc.CachedUserAgent(),
		"config":        cfg,
		"patcher":       p.patcher.Status(),
		"accounts":      accs,
		"refreshed_at":  refreshedAt,
		"refresh_error": refreshErr,
	}
}

func jsonResponse(status int, v any) abi.ManagementResponse {
	raw, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"error":"encode response failed"}`)
	}
	return abi.ManagementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type":  []string{"application/json; charset=utf-8"},
			"Cache-Control": []string{"no-store"},
		},
		Body: raw,
	}
}
