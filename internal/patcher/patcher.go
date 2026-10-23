// Package patcher hot-patches CPA's static/management.html so that the built-in
// Cli-Proxy-API-Management-Center UI displays Antigravity Gemini quota groups.
package patcher

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	PatchMarkerStart = "<!-- cpa-antigravity-category-plugin:start -->"
	PatchMarkerEnd   = "<!-- cpa-antigravity-category-plugin:end -->"

	// QuotaSummaryRoute is the plugin management route the injected script reroutes
	// Antigravity retrieveUserQuotaSummary api-call requests to.
	QuotaSummaryRoute = "/v0/management/antigravity-category-plugin/quota-summary"

	quotaURLDaily        = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	quotaURLDailySandbox = "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary"
	quotaURLProd         = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
)

// urlArrayVariants holds the original and reordered ANTIGRAVITY_QUOTA_URLS literals for
// every quote style a bundler may emit (Vite/Rolldown currently emits backticks).
var urlArrayVariants = func() [][2]string {
	var out [][2]string
	for _, q := range []string{"`", `"`, `'`} {
		join := func(urls ...string) string {
			return "[" + q + strings.Join(urls, q+","+q) + q + "]"
		}
		out = append(out, [2]string{
			join(quotaURLDaily, quotaURLDailySandbox, quotaURLProd),
			join(quotaURLProd, quotaURLDaily, quotaURLDailySandbox),
		})
	}
	return out
}()

var metaCharsetRe = regexp.MustCompile(`(?i)<meta[^>]*charset[^>]*>`)

// Status describes the current state of the management.html patcher.
type Status struct {
	Enabled       bool     `json:"enabled"`
	TargetPath    string   `json:"target_path,omitempty"`
	CandidateList []string `json:"candidate_list,omitempty"`
	Patched       bool     `json:"patched"`
	LastCheckedAt string   `json:"last_checked_at,omitempty"`
	LastPatchedAt string   `json:"last_patched_at,omitempty"`
	UserAgent     string   `json:"user_agent,omitempty"`
	Message       string   `json:"message,omitempty"`
}

// Patcher watches and patches management.html in the CPA static directory.
type Patcher struct {
	mu            sync.Mutex
	enabled       bool
	candidates    []string
	userAgentFunc func() string
	status        Status
}

// New creates a new Patcher.
func New(enabled bool, candidates []string, userAgentFunc func() string) *Patcher {
	p := &Patcher{
		enabled:       enabled,
		candidates:    candidates,
		userAgentFunc: userAgentFunc,
		status: Status{
			Enabled:       enabled,
			CandidateList: candidates,
		},
	}
	return p
}

// Configure updates the patcher's configuration.
func (p *Patcher) Configure(enabled bool, candidates []string) {
	p.mu.Lock()
	p.enabled = enabled
	p.candidates = candidates
	p.status.Enabled = enabled
	p.status.CandidateList = candidates
	p.mu.Unlock()
}

// Status returns a snapshot of the patcher status.
func (p *Patcher) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.status
	if p.userAgentFunc != nil {
		st.UserAgent = p.userAgentFunc()
	}
	return st
}

// EnsurePatched locates management.html and applies the Antigravity quota fix if needed.
func (p *Patcher) EnsurePatched() Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	p.status.LastCheckedAt = now
	ua := "antigravity/hub/2.24.0 (aidev_client; os_type=darwin; arch=arm64; cl=1000335682)"
	if p.userAgentFunc != nil {
		if v := strings.TrimSpace(p.userAgentFunc()); v != "" {
			ua = v
		}
	}
	p.status.UserAgent = ua

	target := findExistingFile(p.candidates)
	if target == "" {
		p.status.TargetPath = ""
		p.status.Patched = false
		p.status.Message = "management.html not found yet in candidate paths"
		return p.status
	}
	p.status.TargetPath = target

	raw, err := os.ReadFile(target)
	if err != nil {
		p.status.Patched = false
		p.status.Message = fmt.Sprintf("read %s failed: %v", target, err)
		return p.status
	}

	content := string(raw)
	patchedContent, changed := ApplyPatchToHTML(content, ua)
	if !changed {
		p.status.Patched = IsPatchedHTML(content)
		p.status.Message = "management.html is already patched"
		return p.status
	}

	// Save a backup of the unpatched original once.
	backupPath := target + ".orig.bak"
	if _, errStat := os.Stat(backupPath); os.IsNotExist(errStat) && !IsPatchedHTML(content) {
		_ = os.WriteFile(backupPath, raw, 0o644)
	}

	if err := atomicWriteFile(target, []byte(patchedContent)); err != nil {
		p.status.Patched = false
		p.status.Message = fmt.Sprintf("write %s failed: %v", target, err)
		return p.status
	}

	p.status.Patched = true
	p.status.LastPatchedAt = now
	p.status.Message = "management.html patched successfully"
	return p.status
}

// Restore removes the injected patch from management.html.
func (p *Patcher) Restore() Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	p.status.LastCheckedAt = now
	target := findExistingFile(p.candidates)
	if target == "" {
		p.status.Message = "management.html not found"
		return p.status
	}
	p.status.TargetPath = target

	raw, err := os.ReadFile(target)
	if err != nil {
		p.status.Message = fmt.Sprintf("read %s failed: %v", target, err)
		return p.status
	}
	restored, changed := RemovePatchFromHTML(string(raw))
	if changed {
		if err := atomicWriteFile(target, []byte(restored)); err != nil {
			p.status.Message = fmt.Sprintf("restore %s failed: %v", target, err)
			return p.status
		}
	}
	p.status.Patched = false
	p.status.Message = "management.html restored to original"
	return p.status
}

// IsPatchedHTML reports whether the HTML string contains the plugin's patch block.
func IsPatchedHTML(htmlContent string) bool {
	return strings.Contains(htmlContent, PatchMarkerStart) && strings.Contains(htmlContent, PatchMarkerEnd)
}

// ApplyPatchToHTML injects or updates the Antigravity quota interceptor script and fixes
// hardcoded URL order in management.html.
func ApplyPatchToHTML(htmlContent, userAgent string) (string, bool) {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "antigravity/hub/2.24.0 (aidev_client; os_type=darwin; arch=arm64; cl=1000335682)"
	}
	out := htmlContent

	// 1. Replace existing patch block if present so script updates propagate cleanly.
	scriptBlock := buildInjectedScriptBlock(userAgent)
	if startIdx := strings.Index(out, PatchMarkerStart); startIdx >= 0 {
		if endIdx := strings.Index(out[startIdx:], PatchMarkerEnd); endIdx >= 0 {
			fullEnd := startIdx + endIdx + len(PatchMarkerEnd)
			out = out[:startIdx] + scriptBlock + out[fullEnd:]
		}
	} else {
		out = injectIntoHead(out, scriptBlock)
	}

	// 2. Reorder hardcoded ANTIGRAVITY_QUOTA_URLS in the bundle so cloudcode-pa.googleapis.com is
	// tried first. This is the fallback path when the plugin route is unreachable.
	for _, v := range urlArrayVariants {
		out = strings.ReplaceAll(out, v[0], v[1])
	}

	return out, out != htmlContent
}

// injectIntoHead places the script block right after <meta charset> (so the charset
// declaration stays within the first 1024 bytes), else after <head>, else at the top.
// Classic scripts there run before the panel's deferred module bundle.
func injectIntoHead(htmlContent, block string) string {
	lower := strings.ToLower(htmlContent)
	headIdx := strings.Index(lower, "<head")
	headEnd := strings.Index(lower, "</head>")
	if headIdx >= 0 {
		searchEnd := len(htmlContent)
		if headEnd > headIdx {
			searchEnd = headEnd
		}
		if loc := metaCharsetRe.FindStringIndex(htmlContent[headIdx:searchEnd]); loc != nil {
			pos := headIdx + loc[1]
			return htmlContent[:pos] + "\n" + block + htmlContent[pos:]
		}
		if closeIdx := strings.IndexByte(htmlContent[headIdx:], '>'); closeIdx >= 0 {
			pos := headIdx + closeIdx + 1
			return htmlContent[:pos] + "\n" + block + htmlContent[pos:]
		}
	}
	return block + "\n" + htmlContent
}

// RemovePatchFromHTML strips the injected script block and restores the original URL order.
func RemovePatchFromHTML(htmlContent string) (string, bool) {
	out := htmlContent
	if startIdx := strings.Index(out, PatchMarkerStart); startIdx >= 0 {
		if endIdx := strings.Index(out[startIdx:], PatchMarkerEnd); endIdx >= 0 {
			fullEnd := startIdx + endIdx + len(PatchMarkerEnd)
			// injectIntoHead adds a single newline either before the block or (at document start) after it.
			before, after := out[:startIdx], out[fullEnd:]
			if strings.HasSuffix(before, "\n") {
				before = strings.TrimSuffix(before, "\n")
			} else {
				after = strings.TrimPrefix(after, "\n")
			}
			out = before + after
		}
	}
	for _, v := range urlArrayVariants {
		out = strings.ReplaceAll(out, v[1], v[0])
	}
	return out, out != htmlContent
}

// atomicWriteFile replaces path via a temp file + rename so CPA never serves a
// truncated management.html while it is being rewritten.
func atomicWriteFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".management.html.agcp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func buildInjectedScriptBlock(userAgent string) string {
	escapedUA := strings.ReplaceAll(userAgent, `\`, `\\`)
	escapedUA = strings.ReplaceAll(escapedUA, `'`, `\'`)
	escapedUA = strings.ReplaceAll(escapedUA, `<`, `\x3c`)
	return PatchMarkerStart + `<script id="cpa-antigravity-category-plugin-patch">
(function(){
  'use strict';
  if (window.__CPA_ANTIGRAVITY_QUOTA_PATCHED__) return;
  window.__CPA_ANTIGRAVITY_QUOTA_PATCHED__ = true;
  var HUB_UA = '` + escapedUA + `';
  var QUOTA_URLS = ['` + quotaURLProd + `', '` + quotaURLDaily + `', '` + quotaURLDailySandbox + `'];
  var API_CALL_RE = /\/v\d+\/management\/(?:requests\/)?api-call\/?$/;
  // Per-endpoint budget; a stuck endpoint is skipped instead of leaving the panel loading.
  var ENDPOINT_TIMEOUT_MS = 20000;
  var origFetch = typeof window.fetch === 'function' ? window.fetch.bind(window) : null;

  function parseURL(u) {
    try { return new URL(String(u), window.location.href); } catch (_) { return null; }
  }
  function isQuotaCall(method, url, body) {
    if (String(method || 'GET').toUpperCase() !== 'POST' || typeof body !== 'string') return false;
    if (body.indexOf(':retrieveUserQuotaSummary') === -1) return false;
    var parsed = parseURL(url);
    return !!parsed && API_CALL_RE.test(parsed.pathname);
  }
  function rememberKey(name, value) {
    if (!/^authorization$/i.test(String(name))) return;
    var m = /^Bearer\s+(.+)$/i.exec(String(value || ''));
    if (m) window.__CPA_AG_MGMT_KEY__ = m[1].trim();
  }
  function poolOf(group) {
    var name = String((group && (group.displayName || group.display_name)) || '').toLowerCase();
    if (name.indexOf('gemini') !== -1) return 'gemini';
    if (name.indexOf('claude') !== -1 || name.indexOf('gpt') !== -1) return 'claude';
    return name;
  }
  function timedFetch(url, init) {
    var ctrl = typeof AbortController === 'function' ? new AbortController() : null;
    if (ctrl) init.signal = ctrl.signal;
    return new Promise(function(resolve, reject) {
      var timer = setTimeout(function() {
        if (ctrl) ctrl.abort();
        reject(new Error('endpoint timeout'));
      }, ENDPOINT_TIMEOUT_MS);
      origFetch(url, init).then(function(resp) {
        return resp.text().then(function(text) { return { ok: resp.ok, text: text }; });
      }).then(function(res) {
        clearTimeout(timer);
        resolve(res);
      }, function(err) {
        clearTimeout(timer);
        reject(err);
      });
    });
  }
  // Replays the panel's own api-call (same CPA proxy, same credential) against every
  // Antigravity endpoint with the Hub User-Agent, merging Gemini + Claude/GPT groups.
  // Only the Hub UA on cloudcode-pa returns the Gemini pool; the panel's CLI UA does not.
  // Resolves with api-call shaped JSON text; rejects so the caller can send the original.
  function mergedQuota(url, headers, body, credentials) {
    if (!origFetch) return Promise.reject(new Error('fetch unavailable'));
    var payload;
    try { payload = JSON.parse(body); } catch (e) { return Promise.reject(e); }
    var base = null, groups = [], seen = {}, lastText = null, i = 0;

    function next() {
      if (i >= QUOTA_URLS.length || (seen.gemini && seen.claude)) return finish();
      var req = {};
      Object.keys(payload).forEach(function(k) { req[k] = payload[k]; });
      req.url = QUOTA_URLS[i++];
      req.header = {};
      Object.keys(payload.header || {}).forEach(function(k) { req.header[k] = payload.header[k]; });
      req.header['User-Agent'] = HUB_UA;
      return timedFetch(url, {
        method: 'POST',
        headers: headers,
        body: JSON.stringify(req),
        credentials: credentials
      }).then(function(res) {
        // api-call itself failed (e.g. 401): let the panel's original request surface it.
        if (!res.ok) throw new Error('api-call HTTP failure');
        var env = JSON.parse(res.text);
        lastText = res.text;
        var code = Number(env.status_code != null ? env.status_code : env.statusCode);
        if (!(code >= 200 && code < 300)) return;
        var raw = env.body != null ? env.body : env.bodyText;
        var data = typeof raw === 'string' ? JSON.parse(raw) : raw;
        if (!data || !Array.isArray(data.groups)) return;
        if (!base) base = { env: env, data: data };
        data.groups.forEach(function(g) {
          var k = poolOf(g);
          if (seen[k]) return;
          seen[k] = true;
          groups.push(g);
        });
      }, function() { /* endpoint unreachable or timed out: try the next one */ }).then(next);
    }
    function finish() {
      if (base && groups.length) {
        groups.sort(function(a, b) { return (poolOf(a) === 'gemini' ? 0 : 1) - (poolOf(b) === 'gemini' ? 0 : 1); });
        base.data.groups = groups;
        base.env.body = JSON.stringify(base.data);
        delete base.env.bodyText;
        return JSON.stringify(base.env);
      }
      if (lastText) return lastText;
      throw new Error('all Antigravity quota endpoints failed');
    }
    return next();
  }

  var XHR = window.XMLHttpRequest;
  if (XHR && XHR.prototype) {
    var proto = XHR.prototype;
    var origOpen = proto.open, origSend = proto.send, origSetHeader = proto.setRequestHeader, origAbort = proto.abort;
    proto.open = function(method, url) {
      this.__agcp = { method: method, url: url, headers: {} };
      return origOpen.apply(this, arguments);
    };
    proto.setRequestHeader = function(name, value) {
      if (this.__agcp) this.__agcp.headers[name] = value;
      rememberKey(name, value);
      return origSetHeader.apply(this, arguments);
    };
    proto.abort = function() {
      if (this.__agcp) this.__agcp.aborted = true;
      return origAbort.apply(this, arguments);
    };
    proto.send = function(body) {
      var st = this.__agcp;
      if (!st || !isQuotaCall(st.method, st.url, body)) return origSend.apply(this, arguments);
      var xhr = this, args = arguments;
      mergedQuota(st.url, st.headers, body, xhr.withCredentials ? 'include' : 'same-origin').then(function(text) {
        if (st.aborted) return;
        completeXHR(xhr, text);
      }, function() {
        if (st.aborted) return;
        origSend.apply(xhr, args);
      });
    };
  }

  function completeXHR(xhr, text) {
    var rt = xhr.responseType;
    var response = text;
    if (rt === 'json') { try { response = JSON.parse(text); } catch (_) { response = null; } }
    var values = { readyState: 4, status: 200, statusText: 'OK', responseText: text, response: response, responseURL: '' };
    Object.keys(values).forEach(function(k) {
      try { Object.defineProperty(xhr, k, { configurable: true, get: function() { return values[k]; } }); } catch (_) {}
    });
    var rawHeaders = 'content-type: application/json; charset=utf-8\r\n';
    xhr.getAllResponseHeaders = function() { return rawHeaders; };
    xhr.getResponseHeader = function(name) {
      return String(name).toLowerCase() === 'content-type' ? 'application/json; charset=utf-8' : null;
    };
    ['readystatechange', 'load', 'loadend'].forEach(function(type) {
      var ev;
      try { ev = type === 'readystatechange' ? new Event(type) : new ProgressEvent(type); } catch (_) { ev = new Event(type); }
      xhr.dispatchEvent(ev);
    });
  }

  if (origFetch) {
    window.fetch = function(input, init) {
      try {
        var url = typeof input === 'string' ? input : (input && input.url) || '';
        var method = (init && init.method) || (input && input.method) || 'GET';
        var body = init && init.body;
        if (isQuotaCall(method, url, body)) {
          var headers = {};
          var src = (init && init.headers) || {};
          if (typeof Headers !== 'undefined' && src instanceof Headers) {
            src.forEach(function(v, k) { headers[k] = v; });
          } else if (Array.isArray(src)) {
            src.forEach(function(pair) { headers[pair[0]] = pair[1]; });
          } else {
            Object.keys(src).forEach(function(k) { headers[k] = src[k]; });
          }
          Object.keys(headers).forEach(function(k) { rememberKey(k, headers[k]); });
          return mergedQuota(url, headers, body, (init && init.credentials) || 'same-origin').then(function(text) {
            return new Response(text, { status: 200, headers: { 'Content-Type': 'application/json; charset=utf-8' } });
          }, function() {
            return origFetch(input, init);
          });
        }
      } catch (_) {}
      return origFetch(input, init);
    };
  }
})();
</script>` + PatchMarkerEnd
}

func findExistingFile(candidates []string) string {
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
