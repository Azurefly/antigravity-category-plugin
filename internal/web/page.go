// Package web provides the embedded HTML dashboard for the Antigravity Category Quota plugin.
package web

// Page returns the HTML content served at /v0/resource/plugins/antigravity-category-plugin/page.
func Page() []byte {
	return []byte(pageHTML)
}

// The page is a Go raw string, so its JavaScript must not use template literals.
const pageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Antigravity Quota</title>
  <style>
    :root {
      --bg: #f6f7f9;
      --surface: #ffffff;
      --surface-2: #f1f3f6;
      --border: #e3e6eb;
      --text: #1b1f24;
      --muted: #6b7280;
      --accent: #2563eb;
      --accent-text: #ffffff;
      --ok: #16a34a;
      --warn: #d97706;
      --bad: #dc2626;
      --track: #e8ebef;
      --ok-soft: rgba(22,163,74,.12);
      --warn-soft: rgba(217,119,6,.12);
      --bad-soft: rgba(220,38,38,.10);
      --accent-soft: rgba(37,99,235,.10);
      --shadow: 0 1px 2px rgba(16,24,40,.04), 0 1px 3px rgba(16,24,40,.06);
    }
    @media (prefers-color-scheme: dark) {
      :root:not([data-theme="light"]) {
        --bg: #0f1115; --surface: #171a21; --surface-2: #1e222b; --border: #2a2f3a;
        --text: #e7e9ee; --muted: #9aa3b2; --accent: #4f8cff; --track: #262b35;
        --ok: #22c55e; --warn: #f59e0b; --bad: #f87171;
        --ok-soft: rgba(34,197,94,.14); --warn-soft: rgba(245,158,11,.14);
        --bad-soft: rgba(248,113,113,.14); --accent-soft: rgba(79,140,255,.14);
        --shadow: none;
      }
    }
    :root[data-theme="dark"] {
      --bg: #0f1115; --surface: #171a21; --surface-2: #1e222b; --border: #2a2f3a;
      --text: #e7e9ee; --muted: #9aa3b2; --accent: #4f8cff; --track: #262b35;
      --ok: #22c55e; --warn: #f59e0b; --bad: #f87171;
      --ok-soft: rgba(34,197,94,.14); --warn-soft: rgba(245,158,11,.14);
      --bad-soft: rgba(248,113,113,.14); --accent-soft: rgba(79,140,255,.14);
      --shadow: none;
    }
    * { box-sizing: border-box; }
    html, body { margin: 0; background: var(--bg); color: var(--text); }
    body {
      padding: 20px 16px 40px;
      font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif;
    }
    .wrap { max-width: 1120px; margin: 0 auto; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; }
    .muted { color: var(--muted); }
    header { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
    h1 { margin: 0; font-size: 20px; font-weight: 650; letter-spacing: -.01em; }
    .sub { color: var(--muted); font-size: 13px; margin-top: 2px; }
    .toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
    button {
      appearance: none; border: 1px solid var(--border); background: var(--surface); color: var(--text);
      padding: 7px 12px; border-radius: 8px; font: inherit; font-size: 13px; font-weight: 550; cursor: pointer;
      display: inline-flex; align-items: center; gap: 6px;
    }
    button:hover { border-color: var(--muted); }
    button.primary { background: var(--accent); border-color: var(--accent); color: var(--accent-text); }
    button:disabled { opacity: .6; cursor: progress; }
    .spin { width: 12px; height: 12px; border: 2px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: spin .8s linear infinite; display: none; }
    .busy .spin { display: inline-block; }
    @keyframes spin { to { transform: rotate(360deg); } }
    .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; box-shadow: var(--shadow); }
    .banner { padding: 12px 14px; margin-bottom: 12px; border-radius: 10px; font-size: 13px; display: none; }
    .banner.show { display: block; }
    .banner.err { background: var(--bad-soft); color: var(--bad); }
    .banner.info { background: var(--accent-soft); color: var(--text); }
    .keybox { display: none; padding: 14px; margin-bottom: 12px; }
    .keybox.show { display: block; }
    .keybox form { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
    .keybox input {
      flex: 1 1 240px; min-width: 0; padding: 7px 10px; border-radius: 8px; border: 1px solid var(--border);
      background: var(--surface-2); color: var(--text); font: inherit;
    }
    .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-bottom: 12px; }
    .stat { padding: 12px 14px; min-width: 0; }
    .stat .label { color: var(--muted); font-size: 12px; }
    .stat .value { font-size: 22px; font-weight: 650; margin-top: 2px; font-variant-numeric: tabular-nums; }
    .stat .hint { color: var(--muted); font-size: 12px; margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .chip { display: inline-flex; align-items: center; gap: 6px; padding: 2px 8px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; }
    .chip::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
    .chip.ok { background: var(--ok-soft); color: var(--ok); }
    .chip.warn { background: var(--warn-soft); color: var(--warn); }
    .chip.bad { background: var(--bad-soft); color: var(--bad); }
    .chip.plain { background: var(--surface-2); color: var(--muted); }
    .chip.plain::before { display: none; }
    .accounts { display: grid; gap: 12px; }
    .acct { padding: 14px 16px; }
    .acct-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 12px; }
    .acct-name { font-weight: 650; font-size: 15px; word-break: break-all; }
    .acct-meta { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
    .pools { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr)); gap: 12px; }
    .pool { background: var(--surface-2); border-radius: 10px; padding: 12px; }
    .pool-title { display: flex; justify-content: space-between; align-items: baseline; font-weight: 600; margin-bottom: 10px; }
    .pool-title .muted { font-weight: 400; font-size: 12px; }
    .pool.missing { border: 1px dashed var(--border); background: transparent; color: var(--muted); font-size: 13px; }
    .bucket + .bucket { margin-top: 10px; }
    .bucket-top { display: flex; justify-content: space-between; gap: 8px; font-size: 13px; margin-bottom: 4px; }
    .pct { font-weight: 650; font-variant-numeric: tabular-nums; }
    .bar { height: 8px; border-radius: 999px; background: var(--track); overflow: hidden; }
    .bar > i { display: block; height: 100%; border-radius: 999px; }
    .reset { color: var(--muted); font-size: 12px; margin-top: 3px; }
    .acct-err { background: var(--bad-soft); color: var(--bad); border-radius: 8px; padding: 8px 10px; font-size: 13px; margin-bottom: 10px; word-break: break-word; }
    .empty { padding: 28px 16px; text-align: center; color: var(--muted); }
    .skeleton { height: 150px; border-radius: 12px; background: linear-gradient(90deg, var(--surface) 0%, var(--surface-2) 50%, var(--surface) 100%); background-size: 200% 100%; animation: shimmer 1.4s infinite; border: 1px solid var(--border); }
    @keyframes shimmer { to { background-position: -200% 0; } }
    details.card { margin-top: 16px; padding: 0 14px; }
    details summary { cursor: pointer; padding: 12px 0; font-weight: 600; display: flex; align-items: center; justify-content: space-between; gap: 8px; list-style: none; }
    details summary::-webkit-details-marker { display: none; }
    .kv { display: grid; grid-template-columns: 140px 1fr; gap: 6px 12px; padding-bottom: 12px; font-size: 13px; }
    .kv dt { color: var(--muted); }
    .kv dd { margin: 0; word-break: break-all; }
    .patch-actions { display: flex; flex-wrap: wrap; gap: 8px; padding-bottom: 14px; }
    @media (max-width: 560px) { .kv { grid-template-columns: 1fr; } .kv dt { margin-top: 6px; } }
  </style>
</head>
<body>
  <div class="wrap">
    <header>
      <div>
        <h1>Antigravity 双池额度</h1>
        <div class="sub">Gemini Models 与 Claude / GPT 分组跨端点合并 · <span id="version" class="mono"></span></div>
      </div>
      <div class="toolbar">
        <span id="updated" class="muted" style="font-size:12px"></span>
        <button id="refreshBtn" class="primary" type="button"><span class="spin"></span><span>刷新额度</span></button>
      </div>
    </header>

    <div id="banner" class="banner" role="status"></div>

    <div id="keybox" class="card keybox">
      <strong>需要 Management Key</strong>
      <div class="muted" style="font-size:13px">未能从管理面板自动获取密钥（或密钥无效）。请输入 CPA 的 Management Key，仅保存在当前标签页。</div>
      <form id="keyForm">
        <input id="keyInput" type="password" autocomplete="off" placeholder="Management Key" />
        <button class="primary" type="submit">连接</button>
      </form>
    </div>

    <div class="stats">
      <div class="card stat"><div class="label">Antigravity 凭证</div><div class="value" id="statAccounts">-</div><div class="hint" id="statAccountsHint">&nbsp;</div></div>
      <div class="card stat"><div class="label">Gemini 最低剩余</div><div class="value" id="statGemini">-</div><div class="hint" id="statGeminiHint">&nbsp;</div></div>
      <div class="card stat"><div class="label">Claude / GPT 最低剩余</div><div class="value" id="statClaude">-</div><div class="hint" id="statClaudeHint">&nbsp;</div></div>
      <div class="card stat"><div class="label">原生面板修补</div><div class="value" style="font-size:15px;margin-top:6px" id="statPatch">-</div><div class="hint" id="statPatchHint">&nbsp;</div></div>
    </div>

    <div id="accounts" class="accounts"><div class="skeleton"></div></div>

    <details class="card" id="patchDetails">
      <summary><span>management.html 修补详情</span><span id="patchChip"></span></summary>
      <dl class="kv">
        <dt>状态信息</dt><dd id="pMessage">-</dd>
        <dt>目标文件</dt><dd id="pPath" class="mono">-</dd>
        <dt>Hub User-Agent</dt><dd id="pUA" class="mono">-</dd>
        <dt>最近检查</dt><dd id="pChecked">-</dd>
        <dt>最近写入</dt><dd id="pPatched">-</dd>
      </dl>
      <div class="patch-actions">
        <button type="button" id="patchBtn"><span class="spin"></span><span>立即修补</span></button>
        <button type="button" id="unpatchBtn"><span class="spin"></span><span>还原原版面板</span></button>
        <span class="muted" style="font-size:12px;align-self:center">修补后需在管理面板强制刷新（Ctrl+F5 / Cmd+Shift+R）。</span>
      </div>
    </details>
  </div>

  <script>
  (function () {
    'use strict';
    var API = '/v0/management/antigravity-category-plugin';
    var MANUAL_KEY = 'cpa_ag_mgmt_key';
    var state = { data: null, key: '', loading: false };

    function $(id) { return document.getElementById(id); }
    function esc(v) {
      return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
      });
    }
    function safe(fn, fallback) { try { return fn(); } catch (_) { return fallback; } }

    // Follow the Management Center theme when embedded, else the OS preference.
    safe(function () {
      var t = window.parent && window.parent !== window && window.parent.document.documentElement.getAttribute('data-theme');
      if (t === 'dark' || t === 'light') document.documentElement.setAttribute('data-theme', t);
    });

    // ---- Management key discovery -------------------------------------------------
    // Mirrors Management Center's reversible localStorage obfuscation (enc::v1:: + XOR).
    function deobfuscate(raw) {
      var prefix = 'enc::v1::';
      if (!raw || raw.indexOf(prefix) !== 0) return raw;
      var key = new TextEncoder().encode('cli-proxy-api-webui::secure-storage|' + location.host + '|' + navigator.userAgent);
      var bin = atob(raw.slice(prefix.length));
      var out = new Uint8Array(bin.length);
      for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i) ^ key[i % key.length];
      return new TextDecoder().decode(out);
    }
    function panelStoredKey() {
      var auth = safe(function () { return JSON.parse(deobfuscate(localStorage.getItem('cli-proxy-auth'))); }, null);
      var k = auth && auth.state && auth.state.managementKey;
      if (typeof k === 'string' && k) return k;
      var legacy = safe(function () { return deobfuscate(localStorage.getItem('managementKey')); }, '');
      legacy = safe(function () { return JSON.parse(legacy); }, legacy);
      return typeof legacy === 'string' ? legacy : '';
    }
    function candidateKeys() {
      var list = [
        safe(function () { return sessionStorage.getItem(MANUAL_KEY); }, ''),
        safe(function () { return window.parent && window.parent !== window ? window.parent.__CPA_AG_MGMT_KEY__ : ''; }, ''),
        safe(panelStoredKey, '')
      ];
      var seen = {};
      return list.filter(function (k) {
        if (!k || typeof k !== 'string' || seen[k]) return false;
        seen[k] = true;
        return true;
      });
    }

    function call(method, path, keys) {
      keys = keys || (state.key ? [state.key] : candidateKeys());
      if (!keys.length) keys = [''];
      var key = keys[0];
      var headers = { 'Content-Type': 'application/json' };
      if (key) { headers.Authorization = 'Bearer ' + key; headers['X-Management-Key'] = key; }
      return fetch(API + path, { method: method, headers: headers, body: method === 'POST' ? '{}' : undefined, cache: 'no-store' })
        .then(function (res) {
          if ((res.status === 401 || res.status === 403) && keys.length > 1) return call(method, path, keys.slice(1));
          return res.text().then(function (text) {
            var json = safe(function () { return JSON.parse(text); }, null);
            if (res.status === 401 || res.status === 403) {
              state.key = '';
              var e = new Error((json && (json.error || json.message)) || 'Management Key 无效或缺失');
              e.auth = true;
              throw e;
            }
            if (!res.ok || !json) throw new Error('HTTP ' + res.status + (json && json.error ? ' · ' + json.error : (text ? ' · ' + text.slice(0, 200) : '')));
            state.key = key;
            return json;
          });
        });
    }

    // ---- Formatting ----------------------------------------------------------------
    function poolKey(name) {
      var s = String(name || '').toLowerCase();
      if (s.indexOf('gemini') !== -1) return 'gemini';
      if (s.indexOf('claude') !== -1 || s.indexOf('gpt') !== -1) return 'claude';
      return 'other';
    }
    function windowLabel(b) {
      var w = String(b.window || '').toLowerCase();
      var d = String(b.description || '');
      if (w === '5h' || /five.?hour|5.?hour/i.test(d)) return '5 小时限额';
      if (w === 'weekly' || w === 'week' || /week/i.test(d)) return '周限额';
      if (w === 'daily' || w === 'day' || /daily|day/i.test(d)) return '日限额';
      return d || w || '限额';
    }
    function pct(f) { return Math.max(0, Math.min(100, Math.round((Number(f) || 0) * 1000) / 10)); }
    function tone(p) { return p <= 15 ? 'bad' : (p <= 40 ? 'warn' : 'ok'); }
    function toneColor(p) { return 'var(--' + tone(p) + ')'; }
    function fmtReset(iso) {
      if (!iso) return '';
      var d = new Date(iso);
      if (isNaN(d.getTime())) return esc(iso);
      var mins = Math.round((d.getTime() - Date.now()) / 60000);
      var abs = d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
      if (mins <= 0) return '已到重置时间 · ' + abs;
      var days = Math.floor(mins / 1440), h = Math.floor((mins % 1440) / 60), m = mins % 60;
      var rel = days > 0 ? days + ' 天 ' + h + ' 小时' : (h > 0 ? h + ' 小时 ' + m + ' 分' : m + ' 分钟');
      return rel + '后重置 · ' + abs;
    }
    function fmtTime(iso) {
      if (!iso) return '-';
      var d = new Date(iso);
      return isNaN(d.getTime()) ? esc(iso) : d.toLocaleString('zh-CN', { hour12: false });
    }

    // ---- Rendering -----------------------------------------------------------------
    function banner(kind, html) {
      var el = $('banner');
      el.className = 'banner' + (kind ? ' show ' + kind : '');
      el.innerHTML = html || '';
    }
    function setBusy(btn, busy) {
      btn.disabled = busy;
      btn.classList.toggle('busy', busy);
    }

    function renderPool(title, group) {
      if (!group) {
        return '<div class="pool missing"><div class="pool-title">' + esc(title) + '</div>所有端点均未返回该分组</div>';
      }
      var buckets = (group.buckets || []).map(function (b) {
        var p = pct(b.remainingFraction);
        return '<div class="bucket">' +
          '<div class="bucket-top"><span>' + esc(windowLabel(b)) + '</span><span class="pct" style="color:' + toneColor(p) + '">' + p + '%</span></div>' +
          '<div class="bar" role="meter" aria-valuemin="0" aria-valuemax="100" aria-valuenow="' + p + '"><i style="width:' + p + '%;background:' + toneColor(p) + '"></i></div>' +
          (b.resetTime ? '<div class="reset">' + fmtReset(b.resetTime) + '</div>' : '') +
        '</div>';
      }).join('');
      return '<div class="pool"><div class="pool-title"><span>' + esc(group.displayName || title) + '</span>' +
        (group.description ? '<span class="muted">' + esc(group.description) + '</span>' : '') + '</div>' + buckets + '</div>';
    }

    function renderAccount(acc) {
      var groups = acc.groups || [];
      var byPool = {};
      var others = [];
      groups.forEach(function (g) {
        var k = poolKey(g.displayName);
        if (k === 'other') others.push(g); else if (!byPool[k]) byPool[k] = g;
      });
      var plan = acc.subscription && (acc.subscription.plan || acc.subscription.tierName);
      var pools = acc.error && !groups.length ? '' :
        renderPool('Gemini Models', byPool.gemini) + renderPool('Claude and GPT models', byPool.claude) +
        others.map(function (g) { return renderPool(g.displayName, g); }).join('');
      return '<div class="card acct">' +
        '<div class="acct-head">' +
          '<div><div class="acct-name">' + esc(acc.email || acc.name || acc.auth_index) + '</div>' +
          '<div class="muted mono">' + esc(acc.project_id ? 'project: ' + acc.project_id : (acc.name || '')) + '</div></div>' +
          '<div class="acct-meta">' +
            (plan ? '<span class="chip plain">' + esc(plan) + '</span>' : '') +
            (acc.error ? '<span class="chip bad">获取失败</span>' : '<span class="chip ok">正常</span>') +
          '</div>' +
        '</div>' +
        (acc.error ? '<div class="acct-err">' + esc(acc.error) + '</div>' : '') +
        (pools ? '<div class="pools">' + pools + '</div>' : '') +
      '</div>';
    }

    function lowest(list, key) {
      var best = null;
      list.forEach(function (acc) {
        (acc.groups || []).forEach(function (g) {
          if (poolKey(g.displayName) !== key) return;
          (g.buckets || []).forEach(function (b) {
            var p = pct(b.remainingFraction);
            if (!best || p < best.p) best = { p: p, who: acc.email || acc.name || acc.auth_index, label: windowLabel(b) };
          });
        });
      });
      return best;
    }

    function render(data) {
      state.data = data;
      var accounts = data.accounts || [];
      var p = data.patcher || {};
      $('version').textContent = 'v' + (data.version || '?');
      $('updated').textContent = data.refreshed_at ? '更新于 ' + fmtTime(data.refreshed_at) : '';

      var failed = accounts.filter(function (a) { return a.error; }).length;
      $('statAccounts').textContent = data.refreshed_at ? String(accounts.length) : '-';
      $('statAccountsHint').textContent = failed ? failed + ' 个获取失败' : (accounts.length ? '全部正常' : ' ');
      [['gemini', 'statGemini'], ['claude', 'statClaude']].forEach(function (pair) {
        var b = lowest(accounts, pair[0]);
        var el = $(pair[1]);
        el.textContent = b ? b.p + '%' : '-';
        el.style.color = b ? toneColor(b.p) : '';
        $(pair[1] + 'Hint').textContent = b ? b.label + ' · ' + b.who : ' ';
      });

      var chip = !p.enabled ? '<span class="chip plain">已关闭</span>' :
        (p.patched ? '<span class="chip ok">已生效</span>' : '<span class="chip warn">未生效</span>');
      $('statPatch').innerHTML = chip;
      $('statPatchHint').textContent = p.target_path ? p.target_path : (p.message || ' ');
      $('patchChip').innerHTML = chip;
      $('pMessage').textContent = p.message || '-';
      $('pPath').textContent = p.target_path || (p.candidate_list || []).join('\n') || '-';
      $('pUA').textContent = data.user_agent || p.user_agent || '-';
      $('pChecked').textContent = fmtTime(p.last_checked_at);
      $('pPatched').textContent = fmtTime(p.last_patched_at);

      var box = $('accounts');
      if (data.refresh_error) {
        banner('err', '刷新失败：' + esc(data.refresh_error));
      }
      if (!data.refreshed_at) {
        if (!state.loading) box.innerHTML = '<div class="card empty">尚未获取额度，点击右上角「刷新额度」。</div>';
        return;
      }
      box.innerHTML = accounts.length
        ? accounts.map(renderAccount).join('')
        : '<div class="card empty">没有可用的 Antigravity 凭证（已禁用的凭证不会显示）。</div>';
    }

    function handleError(err) {
      if (err && err.auth) {
        $('keybox').classList.add('show');
        banner('', '');
        if (!state.data) $('accounts').innerHTML = '';
        return;
      }
      banner('err', '请求失败：' + esc(err && err.message ? err.message : err));
    }

    function refresh() {
      var btn = $('refreshBtn');
      state.loading = true;
      setBusy(btn, true);
      if (!state.data || !state.data.refreshed_at) $('accounts').innerHTML = '<div class="skeleton"></div><div class="skeleton"></div>';
      banner('info', '正在查询 cloudcode-pa / daily-cloudcode-pa 各端点额度…');
      return call('POST', '/refresh').then(function (data) {
        $('keybox').classList.remove('show');
        banner('', '');
        state.loading = false;
        render(data);
      }).catch(function (err) {
        state.loading = false;
        handleError(err);
        if (state.data) render(state.data);
      }).then(function () { setBusy(btn, false); });
    }

    function patchAction(path, btn) {
      setBusy(btn, true);
      call('POST', path).then(function (data) {
        render(data);
        banner('info', esc((data.patcher && data.patcher.message) || '完成') + '。请在管理面板强制刷新页面。');
      }).catch(handleError).then(function () { setBusy(btn, false); });
    }

    $('refreshBtn').addEventListener('click', refresh);
    $('patchBtn').addEventListener('click', function () { patchAction('/patch', this); });
    $('unpatchBtn').addEventListener('click', function () {
      if (confirm('确定还原原版 management.html？若开启了 auto_patch_panel，插件会在 15 秒内重新修补。')) patchAction('/unpatch', this);
    });
    $('keyForm').addEventListener('submit', function (e) {
      e.preventDefault();
      var k = $('keyInput').value.trim();
      if (!k) return;
      safe(function () { sessionStorage.setItem(MANUAL_KEY, k); });
      state.key = k;
      $('keyInput').value = '';
      refresh();
    });

    // Keep reset countdowns current without refetching.
    setInterval(function () { if (state.data && !state.loading) render(state.data); }, 30000);

    // Opening the page only shows the last cached result; querying every credential
    // upstream is slow, so it runs only when the user clicks 「刷新额度」.
    call('GET', '/status').then(render).catch(handleError);
  })();
  </script>
</body>
</html>
`
