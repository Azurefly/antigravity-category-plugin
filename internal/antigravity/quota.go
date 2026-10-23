// Package antigravity implements Antigravity Hub version resolution, OAuth token
// refresh, project discovery, and multi-endpoint quota group merging.
package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
	"github.com/Azurefly/antigravity-category-plugin/internal/config"
	"github.com/Azurefly/antigravity-category-plugin/internal/host"
)

const (
	HubManifestURL = "https://antigravity-hub-auto-updater-974169037036.us-central1.run.app/manifest/latest-arm64-mac.yml"

	QuotaURLProd         = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	QuotaURLDaily        = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	QuotaURLDailySandbox = "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary"

	LoadCodeAssistURLDaily = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	LoadCodeAssistURLProd  = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"

	OAuthTokenURL = "https://oauth2.googleapis.com/token"

	hubVersionTTL   = 3 * time.Hour
	hubVersionRetry = 30 * time.Minute
)

func defaultOAuthClientID() string {
	parts := []string{"1071006060591", "-tmhssin2h21lcre235vtolojh4g403ep", ".apps.googleusercontent.com"}
	return strings.Join(parts, "")
}

func defaultOAuthClientSecret() string {
	parts := []string{"GOC", "SPX-", "K58FWR486LdLJ", "1mLB8sXC4z6qDAf"}
	return strings.Join(parts, "")
}

var hubChangelists = map[string]string{
	"2.23.0": "998057262",
	"2.24.0": "1000335682",
}

var semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// BuildUserAgent constructs the Antigravity Hub User-Agent aligned with CPA's
// internal/misc/antigravity_version.go.
func BuildUserAgent(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = config.DefaultFallbackHubVer
	}
	meta := "aidev_client; os_type=darwin; arch=arm64"
	if cl := hubChangelists[version]; cl != "" {
		meta += "; cl=" + cl
	}
	return fmt.Sprintf("antigravity/hub/%s (%s)", version, meta)
}

// Service manages Antigravity quota queries across credentials and endpoints.
type Service struct {
	host host.Host

	mu         sync.RWMutex
	cfg        config.Config
	hubVer     string
	hubVerNext time.Time
}

// NewService creates a new Antigravity quota service.
func NewService(h host.Host, cfg config.Config) *Service {
	return &Service{
		host:   h,
		cfg:    cfg,
		hubVer: config.DefaultFallbackHubVer,
	}
}

// SetConfig updates the runtime configuration.
func (s *Service) SetConfig(cfg config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// CurrentUserAgent returns the active User-Agent string (refreshing the Hub manifest if due).
func (s *Service) CurrentUserAgent(ctx context.Context) string {
	s.mu.RLock()
	customUA := strings.TrimSpace(s.cfg.UserAgent)
	ver := s.hubVer
	due := time.Now().After(s.hubVerNext)
	s.mu.RUnlock()

	if customUA != "" {
		return customUA
	}
	if due {
		s.mu.Lock()
		if time.Now().After(s.hubVerNext) {
			s.hubVerNext = time.Now().Add(hubVersionRetry)
			s.mu.Unlock()
			if fetched, err := s.fetchManifestVersion(ctx); err == nil && fetched != "" {
				s.mu.Lock()
				s.hubVer = fetched
				s.hubVerNext = time.Now().Add(hubVersionTTL)
				ver = fetched
				s.mu.Unlock()
			}
		} else {
			ver = s.hubVer
			s.mu.Unlock()
		}
	}
	if ver == "" {
		ver = config.DefaultFallbackHubVer
	}
	return BuildUserAgent(ver)
}

// CachedUserAgent returns the current User-Agent without making network calls.
func (s *Service) CachedUserAgent() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.cfg.UserAgent) != "" {
		return strings.TrimSpace(s.cfg.UserAgent)
	}
	return BuildUserAgent(s.hubVer)
}

func (s *Service) fetchManifestVersion(ctx context.Context) (string, error) {
	if s.host == nil {
		return "", errors.New("host unavailable")
	}
	resp, err := s.host.HTTPDo(ctx, abi.HTTPRequest{
		Method: http.MethodGet,
		URL:    HubManifestURL,
		Headers: http.Header{
			"User-Agent":    []string{"electron-builder"},
			"Cache-Control": []string{"no-cache"},
		},
	}, 8*time.Second)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest status %d", resp.StatusCode)
	}
	for _, line := range strings.Split(string(resp.Body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
			v = strings.Trim(v, `"'`)
			if semverRe.MatchString(v) {
				return v, nil
			}
		}
	}
	return "", errors.New("version not found in manifest")
}

// QuotaURLs returns the ordered list of upstream quota endpoints.
func (s *Service) QuotaURLs() []string {
	s.mu.RLock()
	preferProd := s.cfg.PreferProdEndpoint
	s.mu.RUnlock()
	if preferProd {
		return []string{
			QuotaURLProd,
			QuotaURLDaily,
			QuotaURLDailySandbox,
		}
	}
	return []string{
		QuotaURLDaily,
		QuotaURLDailySandbox,
		QuotaURLProd,
	}
}

// AccountQuotaResult represents the detailed quota state of a single Antigravity credential.
type AccountQuotaResult struct {
	AuthIndex      string                  `json:"auth_index"`
	Name           string                  `json:"name"`
	Email          string                  `json:"email,omitempty"`
	ProjectID      string                  `json:"project_id,omitempty"`
	Disabled       bool                    `json:"disabled,omitempty"`
	Subscription   *abi.QuotaSubscription  `json:"subscription,omitempty"`
	Groups         []abi.QuotaGroup        `json:"groups,omitempty"`
	EndpointsTried []string                `json:"endpoints_tried,omitempty"`
	UserAgent      string                  `json:"user_agent,omitempty"`
	FetchedAt      string                  `json:"fetched_at"`
	Error          string                  `json:"error,omitempty"`
	RawPayload     map[string]any          `json:"raw_payload,omitempty"`
}

// FetchQuotaForAuth implements the CPA QuotaProvider.FetchQuota logic for a single credential.
func (s *Service) FetchQuotaForAuth(ctx context.Context, req abi.QuotaFetchRequest) (abi.QuotaFetchResponse, error) {
	detail, err := s.FetchAccountDetail(ctx, req.AuthIndex, req.StorageJSON, req.Metadata, req.Attributes)
	if err != nil {
		return abi.QuotaFetchResponse{}, err
	}
	return abi.QuotaFetchResponse{
		Subscription: detail.Subscription,
		Groups:       detail.Groups,
	}, nil
}

// FetchAccountDetail resolves credentials, refreshes token if needed, discovers project/subscription,
// queries the quota endpoints, and merges Gemini + Claude/GPT quota groups.
func (s *Service) FetchAccountDetail(
	ctx context.Context,
	authIndex string,
	storageJSON []byte,
	metadata map[string]any,
	attributes map[string]string,
) (AccountQuotaResult, error) {
	res := AccountQuotaResult{
		AuthIndex: authIndex,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	var fileName string
	var parsedStorage map[string]any
	if len(storageJSON) == 0 && authIndex != "" && s.host != nil {
		if got, err := s.host.AuthGet(authIndex); err == nil {
			fileName = got.Name
			res.Name = got.Name
			storageJSON = got.JSON
		}
	}
	if len(storageJSON) > 0 {
		_ = json.Unmarshal(storageJSON, &parsedStorage)
	}

	res.Email = firstNonEmpty(
		mapString(metadata, "email"),
		mapString(parsedStorage, "email"),
		attributes["email"],
	)
	if res.Name == "" {
		res.Name = firstNonEmpty(res.Email, authIndex)
	}

	ua := firstNonEmpty(
		attributes["user_agent"],
		mapString(metadata, "user_agent"),
		mapString(parsedStorage, "user_agent"),
	)
	if ua == "" || strings.Contains(ua, "antigravity/cli/") || strings.HasSuffix(ua, "darwin/arm64") {
		ua = s.CurrentUserAgent(ctx)
	}
	res.UserAgent = ua

	accessToken, errToken := s.resolveAccessToken(ctx, fileName, parsedStorage, metadata, attributes)
	if errToken != nil {
		res.Error = errToken.Error()
		return res, errToken
	}

	projectID := resolveProjectID(parsedStorage, metadata, attributes)
	sub, discoveredProjectID := s.fetchCodeAssistInfo(ctx, accessToken, ua)
	if projectID == "" && discoveredProjectID != "" {
		projectID = discoveredProjectID
	}
	res.ProjectID = projectID
	res.Subscription = sub

	if projectID == "" {
		err := errors.New("missing Antigravity project_id (loadCodeAssist did not return a project)")
		res.Error = err.Error()
		return res, err
	}

	q := s.queryQuotaEndpoints(ctx, accessToken, ua, projectID)
	res.EndpointsTried = q.endpointsTried
	if len(q.groups) == 0 {
		res.Error = q.err.Error()
		return res, q.err
	}

	res.Groups = q.groups
	res.RawPayload = map[string]any{
		"groups": q.rawGroups,
	}
	return res, nil
}

// quotaQueryResult is the outcome of querying the retrieveUserQuotaSummary endpoints.
type quotaQueryResult struct {
	groups         []abi.QuotaGroup
	rawGroups      []any
	basePayload    map[string]any
	baseHeaders    http.Header
	endpointsTried []string

	// Last upstream failure, used when no endpoint returned usable groups.
	err            error
	lastStatusCode int
	lastHeaders    http.Header
	lastBody       []byte
}

// queryQuotaEndpoints walks the configured endpoints and merges Gemini + Claude/GPT groups.
// Returned groups and rawGroups are sorted Gemini first and stay index-aligned.
func (s *Service) queryQuotaEndpoints(ctx context.Context, accessToken, ua, projectID string) quotaQueryResult {
	s.mu.RLock()
	mergeGroups := s.cfg.MergeEndpointGroups
	s.mu.RUnlock()

	reqBody, _ := json.Marshal(map[string]string{"project": projectID})
	var out quotaQueryResult

	for _, endpoint := range s.QuotaURLs() {
		out.endpointsTried = append(out.endpointsTried, endpoint)
		resp, err := s.host.HTTPDo(ctx, abi.HTTPRequest{
			Method: http.MethodPost,
			URL:    endpoint,
			Headers: http.Header{
				"Authorization": []string{"Bearer " + accessToken},
				"Content-Type":  []string{"application/json"},
				"User-Agent":    []string{ua},
			},
			Body: reqBody,
		}, 15*time.Second)
		if err != nil {
			out.err = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			out.err = fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, endpoint, strings.TrimSpace(string(resp.Body)))
			out.lastStatusCode, out.lastHeaders, out.lastBody = resp.StatusCode, resp.Headers, resp.Body
			continue
		}

		groups, rawList, errParse := ParseQuotaSummaryGroups(resp.Body)
		if errParse != nil || len(groups) == 0 {
			if errParse != nil {
				out.err = errParse
			}
			continue
		}
		if out.basePayload == nil {
			_ = json.Unmarshal(resp.Body, &out.basePayload)
			out.baseHeaders = resp.Headers
		}

		out.groups, out.rawGroups = MergeQuotaGroups(out.groups, out.rawGroups, groups, rawList)

		// Stop early if merging is disabled OR if we already have both Gemini and Claude/GPT groups.
		if !mergeGroups || (HasGeminiGroup(out.groups) && HasClaudeGroup(out.groups)) {
			break
		}
	}

	if len(out.groups) == 0 && out.err == nil {
		out.err = errors.New("Antigravity quota endpoints returned no recognizable quota groups")
	}
	sortGroupsWithRaw(out.groups, out.rawGroups)
	return out
}

// QuotaSummaryResponse mirrors CPA's /v0/management/api-call response shape so the
// Management Center can consume it unchanged.
type QuotaSummaryResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

// FetchQuotaSummary answers a Management Center retrieveUserQuotaSummary api-call with a
// single upstream-shaped payload whose groups are merged across all endpoints.
// An error means the plugin could not act on the credential and the caller should fall back.
func (s *Service) FetchQuotaSummary(ctx context.Context, authIndex, projectID string) (QuotaSummaryResponse, error) {
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return QuotaSummaryResponse{}, errors.New("auth_index is required")
	}
	if s.host == nil {
		return QuotaSummaryResponse{}, errors.New("host unavailable")
	}
	got, err := s.host.AuthGet(authIndex)
	if err != nil {
		return QuotaSummaryResponse{}, fmt.Errorf("load credential %s: %w", authIndex, err)
	}
	var storage map[string]any
	if len(got.JSON) > 0 {
		_ = json.Unmarshal(got.JSON, &storage)
	}

	ua := mapString(storage, "user_agent")
	if ua == "" || strings.Contains(ua, "antigravity/cli/") || strings.HasSuffix(ua, "darwin/arm64") {
		ua = s.CurrentUserAgent(ctx)
	}

	accessToken, err := s.resolveAccessToken(ctx, got.Name, storage, nil, nil)
	if err != nil {
		return QuotaSummaryResponse{}, err
	}

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = resolveProjectID(storage, nil, nil)
	}
	if projectID == "" {
		_, projectID = s.fetchCodeAssistInfo(ctx, accessToken, ua)
	}
	if projectID == "" {
		return QuotaSummaryResponse{}, errors.New("missing Antigravity project_id")
	}

	q := s.queryQuotaEndpoints(ctx, accessToken, ua, projectID)
	if len(q.groups) == 0 {
		if q.lastStatusCode == 0 {
			return QuotaSummaryResponse{}, q.err
		}
		// Surface the upstream failure exactly like api-call would.
		return QuotaSummaryResponse{
			StatusCode: q.lastStatusCode,
			Header:     passthroughHeaders(q.lastHeaders),
			Body:       string(q.lastBody),
		}, nil
	}

	payload := q.basePayload
	if payload == nil {
		payload = map[string]any{}
	}
	payload["groups"] = q.rawGroups
	body, err := json.Marshal(payload)
	if err != nil {
		return QuotaSummaryResponse{}, err
	}
	return QuotaSummaryResponse{
		StatusCode: http.StatusOK,
		Header:     passthroughHeaders(q.baseHeaders),
		Body:       string(body),
	}, nil
}

// passthroughHeaders keeps the upstream headers the Management Center reads (Date drives
// its server clock offset for reset countdowns).
func passthroughHeaders(h http.Header) map[string][]string {
	out := map[string][]string{}
	for _, key := range []string{"Date", "Content-Type"} {
		if v := h.Values(key); len(v) > 0 {
			out[key] = v
		}
	}
	return out
}

func sortGroupsWithRaw(groups []abi.QuotaGroup, raw []any) {
	if len(raw) != len(groups) {
		SortQuotaGroups(groups)
		return
	}
	idx := make([]int, len(groups))
	for i := range idx {
		idx[i] = i
	}
	order := func(name string) int {
		switch normalizeGroupKey(name) {
		case "gemini":
			return 0
		case "claude-gpt":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ga, gb := groups[idx[a]], groups[idx[b]]
		oa, ob := order(ga.DisplayName), order(gb.DisplayName)
		if oa != ob {
			return oa < ob
		}
		return ga.DisplayName < gb.DisplayName
	})
	sortedGroups := make([]abi.QuotaGroup, len(groups))
	sortedRaw := make([]any, len(raw))
	for i, j := range idx {
		sortedGroups[i] = groups[j]
		sortedRaw[i] = raw[j]
	}
	copy(groups, sortedGroups)
	copy(raw, sortedRaw)
}

// ParseQuotaSummaryGroups parses an upstream retrieveUserQuotaSummary response body.
func ParseQuotaSummaryGroups(raw []byte) ([]abi.QuotaGroup, []any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}
	rawGroups, _ := payload["groups"].([]any)
	if len(rawGroups) == 0 {
		return nil, nil, nil
	}

	var out []abi.QuotaGroup
	var keptRaw []any

	for i, item := range rawGroups {
		gm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		displayName := firstNonEmpty(
			mapString(gm, "displayName"),
			mapString(gm, "display_name"),
			fmt.Sprintf("Quota Group %d", i+1),
		)
		groupDesc := mapString(gm, "description")
		rawBuckets, _ := gm["buckets"].([]any)

		var buckets []abi.QuotaBucket
		for _, rb := range rawBuckets {
			bm, ok := rb.(map[string]any)
			if !ok {
				continue
			}
			frac, okFrac := parseFraction(bm["remainingFraction"], bm["remaining_fraction"])
			if !okFrac {
				continue
			}
			win := mapString(bm, "window")
			label := firstNonEmpty(
				mapString(bm, "displayName"),
				mapString(bm, "display_name"),
				mapString(bm, "description"),
				win,
			)
			resetTime := firstNonEmpty(
				mapString(bm, "resetTime"),
				mapString(bm, "reset_time"),
			)
			buckets = append(buckets, abi.QuotaBucket{
				Window:            win,
				RemainingFraction: frac,
				ResetTime:         resetTime,
				Description:       label,
			})
		}

		if len(buckets) == 0 {
			continue
		}
		sortBuckets(buckets)
		out = append(out, abi.QuotaGroup{
			DisplayName: displayName,
			Description: groupDesc,
			Buckets:     buckets,
		})
		keptRaw = append(keptRaw, gm)
	}
	return out, keptRaw, nil
}

// MergeQuotaGroups merges newly fetched quota groups into existing groups by normalized displayName.
func MergeQuotaGroups(
	existing []abi.QuotaGroup,
	existingRaw []any,
	incoming []abi.QuotaGroup,
	incomingRaw []any,
) ([]abi.QuotaGroup, []any) {
	seen := make(map[string]int, len(existing))
	for idx, g := range existing {
		seen[normalizeGroupKey(g.DisplayName)] = idx
	}
	for i, g := range incoming {
		key := normalizeGroupKey(g.DisplayName)
		if idx, ok := seen[key]; ok {
			if len(g.Buckets) > len(existing[idx].Buckets) {
				existing[idx] = g
				if i < len(incomingRaw) && idx < len(existingRaw) {
					existingRaw[idx] = incomingRaw[i]
				}
			}
			continue
		}
		seen[key] = len(existing)
		existing = append(existing, g)
		if i < len(incomingRaw) {
			existingRaw = append(existingRaw, incomingRaw[i])
		}
	}
	return existing, existingRaw
}

func normalizeGroupKey(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(lower, "gemini") {
		return "gemini"
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
		return "claude-gpt"
	}
	return lower
}

func HasGeminiGroup(groups []abi.QuotaGroup) bool {
	for _, g := range groups {
		if normalizeGroupKey(g.DisplayName) == "gemini" {
			return true
		}
	}
	return false
}

func HasClaudeGroup(groups []abi.QuotaGroup) bool {
	for _, g := range groups {
		if normalizeGroupKey(g.DisplayName) == "claude-gpt" {
			return true
		}
	}
	return false
}

// SortQuotaGroups places Gemini Models first, Claude and GPT models second.
func SortQuotaGroups(groups []abi.QuotaGroup) {
	order := func(name string) int {
		switch normalizeGroupKey(name) {
		case "gemini":
			return 0
		case "claude-gpt":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		oi, oj := order(groups[i].DisplayName), order(groups[j].DisplayName)
		if oi != oj {
			return oi < oj
		}
		return groups[i].DisplayName < groups[j].DisplayName
	})
}

func sortBuckets(buckets []abi.QuotaBucket) {
	winOrder := func(b abi.QuotaBucket) int {
		w := strings.ToLower(strings.TrimSpace(b.Window))
		d := strings.ToLower(strings.TrimSpace(b.Description))
		if w == "5h" || w == "five-hour" || w == "five_hour" || strings.Contains(d, "5 hour") || strings.Contains(d, "five hour") {
			return 0
		}
		if w == "weekly" || w == "week" || strings.Contains(d, "weekly") {
			return 1
		}
		return 2
	}
	sort.SliceStable(buckets, func(i, j int) bool {
		oi, oj := winOrder(buckets[i]), winOrder(buckets[j])
		if oi != oj {
			return oi < oj
		}
		return buckets[i].Description < buckets[j].Description
	})
}

func (s *Service) resolveAccessToken(
	ctx context.Context,
	fileName string,
	storage map[string]any,
	metadata map[string]any,
	attributes map[string]string,
) (string, error) {
	token := firstNonEmpty(
		mapString(metadata, "access_token"),
		mapString(metadata, "accessToken"),
		mapString(storage, "access_token"),
		mapString(storage, "accessToken"),
		attributes["access_token"],
		attributes["api_key"],
	)
	refreshToken := firstNonEmpty(
		mapString(metadata, "refresh_token"),
		mapString(metadata, "refreshToken"),
		mapString(storage, "refresh_token"),
		mapString(storage, "refreshToken"),
	)

	expired := isTokenExpired(storage, metadata)
	if token != "" && (!expired || refreshToken == "") {
		return token, nil
	}
	if refreshToken == "" {
		return "", errors.New("missing Antigravity access_token and refresh_token")
	}

	clientID := firstNonEmpty(
		mapString(metadata, "client_id"),
		mapString(storage, "client_id"),
		defaultOAuthClientID(),
	)
	clientSecret := firstNonEmpty(
		mapString(metadata, "client_secret"),
		mapString(storage, "client_secret"),
		defaultOAuthClientSecret(),
	)

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	resp, err := s.host.HTTPDo(ctx, abi.HTTPRequest{
		Method: http.MethodPost,
		URL:    OAuthTokenURL,
		Headers: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
		},
		Body: []byte(form.Encode()),
	}, 15*time.Second)
	if err != nil {
		if token != "" {
			return token, nil
		}
		return "", fmt.Errorf("refresh token failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if token != "" {
			return token, nil
		}
		return "", fmt.Errorf("refresh token HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(resp.Body, &tokenResp); err != nil || strings.TrimSpace(tokenResp.AccessToken) == "" {
		if token != "" {
			return token, nil
		}
		return "", errors.New("refresh token response missing access_token")
	}

	newToken := strings.TrimSpace(tokenResp.AccessToken)
	if fileName != "" && strings.HasSuffix(strings.ToLower(fileName), ".json") && storage != nil && s.host != nil {
		now := time.Now()
		storage["access_token"] = newToken
		if strings.TrimSpace(tokenResp.RefreshToken) != "" {
			storage["refresh_token"] = strings.TrimSpace(tokenResp.RefreshToken)
		}
		if tokenResp.ExpiresIn > 0 {
			storage["expires_in"] = tokenResp.ExpiresIn
			storage["timestamp"] = now.UnixMilli()
			storage["expired"] = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
		}
		if updatedRaw, err := json.Marshal(storage); err == nil {
			_, _ = s.host.AuthSave(fileName, updatedRaw)
		}
	}
	return newToken, nil
}

func isTokenExpired(storage, metadata map[string]any) bool {
	expStr := firstNonEmpty(mapString(metadata, "expired"), mapString(storage, "expired"))
	if expStr != "" {
		if ts, err := time.Parse(time.RFC3339, expStr); err == nil {
			return !ts.After(time.Now().Add(30 * time.Second))
		}
	}
	return false
}

func resolveProjectID(storage, metadata map[string]any, attributes map[string]string) string {
	if p := firstNonEmpty(
		mapString(metadata, "project_id"),
		mapString(metadata, "projectId"),
		attributes["project_id"],
		attributes["projectId"],
		attributes["gemini_virtual_project"],
		mapString(storage, "project_id"),
		mapString(storage, "projectId"),
	); p != "" {
		return p
	}
	for _, subKey := range []string{"installed", "web", "metadata", "attributes"} {
		if sub, ok := storage[subKey].(map[string]any); ok {
			if p := firstNonEmpty(mapString(sub, "project_id"), mapString(sub, "projectId")); p != "" {
				return p
			}
		}
	}
	return ""
}

func (s *Service) fetchCodeAssistInfo(ctx context.Context, accessToken, ua string) (*abi.QuotaSubscription, string) {
	reqBody := []byte(`{"metadata":{"ideType":"ANTIGRAVITY"}}`)
	for _, endpoint := range []string{LoadCodeAssistURLDaily, LoadCodeAssistURLProd} {
		resp, err := s.host.HTTPDo(ctx, abi.HTTPRequest{
			Method: http.MethodPost,
			URL:    endpoint,
			Headers: http.Header{
				"Authorization": []string{"Bearer " + accessToken},
				"Content-Type":  []string{"application/json"},
				"User-Agent":    []string{ua},
			},
			Body: reqBody,
		}, 10*time.Second)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(resp.Body, &payload); err != nil {
			continue
		}
		projectID := mapString(payload, "cloudaicompanionProject")
		sub := parseSubscription(payload)
		if projectID != "" || sub != nil {
			return sub, projectID
		}
	}
	return nil, ""
}

func parseSubscription(payload map[string]any) *abi.QuotaSubscription {
	for _, key := range []string{"paidTier", "currentTier"} {
		tier, ok := payload[key].(map[string]any)
		if !ok {
			continue
		}
		tierID := mapString(tier, "id")
		tierName := mapString(tier, "name")
		if tierID == "" && tierName == "" {
			continue
		}
		plan := derivePlanName(tierID, tierName)
		return &abi.QuotaSubscription{
			Plan:     plan,
			TierName: tierName,
			TierID:   tierID,
		}
	}
	return nil
}

func derivePlanName(tierID, tierName string) string {
	combined := strings.ToLower(tierID + " " + tierName)
	switch {
	case strings.Contains(combined, "ultra"):
		return "Ultra"
	case strings.Contains(combined, "pro") || strings.Contains(combined, "standard"):
		return "Pro"
	case strings.Contains(combined, "free"):
		return "Free"
	default:
		if tierName != "" {
			return tierName
		}
		return tierID
	}
}

func parseFraction(values ...any) (float64, bool) {
	for _, raw := range values {
		if raw == nil {
			continue
		}
		switch v := raw.(type) {
		case float64:
			return clamp01(v), true
		case float32:
			return clamp01(float64(v)), true
		case int:
			return clamp01(float64(v)), true
		case int64:
			return clamp01(float64(v)), true
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return clamp01(f), true
			}
		}
	}
	return 0, false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
