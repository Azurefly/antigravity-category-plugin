// Package config parses plugin configuration and resolves host paths.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	PluginID             = "antigravity-category-plugin"
	DefaultFallbackHubVer = "2.24.0"
	DefaultFallbackHubCL  = "1000335682"
)

// Config holds runtime settings for antigravity-category-plugin.
type Config struct {
	// AutoPatchPanel enables automatic hot-patching of management.html so that
	// the built-in Management Center UI displays Gemini quota groups properly.
	AutoPatchPanel bool `json:"auto_patch_panel"`
	// ManagementHTMLPath optionally overrides the path to management.html.
	ManagementHTMLPath string `json:"management_html_path"`
	// UserAgent optionally overrides the Antigravity Hub User-Agent string.
	UserAgent string `json:"user_agent"`
	// PreferProdEndpoint prioritizes cloudcode-pa.googleapis.com over daily-cloudcode-pa.
	PreferProdEndpoint bool `json:"prefer_prod_endpoint"`
	// MergeEndpointGroups merges quota groups across all 3 Antigravity endpoints
	// when a single endpoint omits the Gemini Models or Claude/GPT group.
	MergeEndpointGroups bool `json:"merge_endpoint_groups"`
}

// Default returns the default plugin configuration.
func Default() Config {
	return Config{
		AutoPatchPanel:      true,
		PreferProdEndpoint:  true,
		MergeEndpointGroups: true,
	}
}

// Parse decodes YAML or JSON configuration bytes into Config, starting from Default().
func Parse(raw []byte) (Config, error) {
	cfg := Default()
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return cfg, nil
	}

	// Try JSON first.
	if strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return cfg, err
		}
		applyMap(&cfg, m)
		return cfg, nil
	}

	// Parse simple YAML key-value lines.
	m := make(map[string]any)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		val = strings.Trim(val, `"'`)
		if b, err := strconv.ParseBool(val); err == nil {
			m[key] = b
		} else {
			m[key] = val
		}
	}
	applyMap(&cfg, m)
	return cfg, nil
}

func applyMap(cfg *Config, m map[string]any) {
	if v, ok := boolVal(m, "auto_patch_panel", "auto-patch-panel"); ok {
		cfg.AutoPatchPanel = v
	}
	if v, ok := stringVal(m, "management_html_path", "management-html-path"); ok {
		cfg.ManagementHTMLPath = v
	}
	if v, ok := stringVal(m, "user_agent", "user-agent"); ok {
		cfg.UserAgent = v
	}
	if v, ok := boolVal(m, "prefer_prod_endpoint", "prefer-prod-endpoint"); ok {
		cfg.PreferProdEndpoint = v
	}
	if v, ok := boolVal(m, "merge_endpoint_groups", "merge-endpoint-groups"); ok {
		cfg.MergeEndpointGroups = v
	}
}

func boolVal(m map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		raw, exists := m[k]
		if !exists || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case bool:
			return v, true
		case string:
			if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				return b, true
			}
		}
	}
	return false, false
}

func stringVal(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		raw, exists := m[k]
		if !exists || raw == nil {
			continue
		}
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s), true
		}
	}
	return "", false
}

// HostConfigPath resolves the CPA config.yaml path from process arguments and workDir.
func HostConfigPath(args []string, workDir string) string {
	for i := 1; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "-config" || arg == "--config" || arg == "-c":
			if i+1 < len(args) {
				return resolvePath(workDir, args[i+1])
			}
		case strings.HasPrefix(arg, "-config="):
			return resolvePath(workDir, strings.TrimPrefix(arg, "-config="))
		case strings.HasPrefix(arg, "--config="):
			return resolvePath(workDir, strings.TrimPrefix(arg, "--config="))
		case strings.HasPrefix(arg, "-c="):
			return resolvePath(workDir, strings.TrimPrefix(arg, "-c="))
		}
	}
	if workDir != "" {
		return filepath.Join(workDir, "config.yaml")
	}
	return "config.yaml"
}

// ManagementHTMLCandidates returns ordered candidate paths for CPA's management.html.
func ManagementHTMLCandidates(customPath, hostConfigPath, workDir string) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		cleaned := filepath.Clean(p)
		if !strings.EqualFold(filepath.Base(cleaned), "management.html") {
			if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
				cleaned = filepath.Join(cleaned, "management.html")
			}
		}
		if _, ok := seen[cleaned]; !ok {
			seen[cleaned] = struct{}{}
			out = append(out, cleaned)
		}
	}

	if customPath != "" {
		add(resolvePath(workDir, customPath))
	}
	if envStatic := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); envStatic != "" {
		if strings.EqualFold(filepath.Base(envStatic), "management.html") {
			add(envStatic)
		} else {
			add(filepath.Join(envStatic, "management.html"))
		}
	}
	for _, envKey := range []string{"WRITABLE_PATH", "CLIPROXY_WRITABLE_PATH"} {
		if w := strings.TrimSpace(os.Getenv(envKey)); w != "" {
			add(filepath.Join(w, "static", "management.html"))
		}
	}
	if hostConfigPath != "" {
		base := filepath.Dir(hostConfigPath)
		if info, err := os.Stat(hostConfigPath); err == nil && info.IsDir() {
			base = hostConfigPath
		}
		add(filepath.Join(base, "static", "management.html"))
	}
	if workDir != "" {
		add(filepath.Join(workDir, "static", "management.html"))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		add(filepath.Join(filepath.Dir(exe), "static", "management.html"))
	}
	add("/CLIProxyAPI/static/management.html")
	return out
}

func resolvePath(workDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) || workDir == "" {
		return p
	}
	return filepath.Join(workDir, p)
}
