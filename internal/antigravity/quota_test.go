package antigravity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
	"github.com/Azurefly/antigravity-category-plugin/internal/config"
)

type fakeHost struct {
	responses map[string]abi.HTTPResponse
	auths     map[string]abi.HostAuthGetResponse
}

func (f *fakeHost) AuthList() ([]abi.HostAuthFileEntry, error) { return nil, nil }
func (f *fakeHost) AuthGet(authIndex string) (abi.HostAuthGetResponse, error) {
	return f.auths[authIndex], nil
}
func (f *fakeHost) AuthSave(name string, rawJSON json.RawMessage) (abi.HostAuthSaveResponse, error) {
	return abi.HostAuthSaveResponse{}, nil
}
func (f *fakeHost) HTTPDo(_ context.Context, req abi.HTTPRequest, _ time.Duration) (abi.HTTPResponse, error) {
	if resp, ok := f.responses[req.URL]; ok {
		return resp, nil
	}
	return abi.HTTPResponse{StatusCode: http.StatusNotFound}, nil
}
func (f *fakeHost) Log(level, message string, fields map[string]any) {}

func TestBuildUserAgent(t *testing.T) {
	ua := BuildUserAgent("2.24.0")
	want := "antigravity/hub/2.24.0 (aidev_client; os_type=darwin; arch=arm64; cl=1000335682)"
	if ua != want {
		t.Fatalf("BuildUserAgent(2.24.0) = %q, want %q", ua, want)
	}

	ua225 := BuildUserAgent("2.25.0")
	if !strings.HasPrefix(ua225, "antigravity/hub/2.25.0 (aidev_client; os_type=darwin; arch=arm64") {
		t.Fatalf("unexpected 2.25.0 UA: %q", ua225)
	}
}

func TestFetchAccountDetailMergesEndpointsWhenDailyOmitsGemini(t *testing.T) {
	fh := splitEndpointHost()

	cfg := config.Default()
	// Even if daily endpoint is tried first, MergeEndpointGroups must continue to QuotaURLProd until Gemini is found!
	cfg.PreferProdEndpoint = false
	cfg.MergeEndpointGroups = true

	svc := NewService(fh, cfg)
	detail, err := svc.FetchAccountDetail(
		context.Background(),
		"idx-1",
		[]byte(`{"access_token":"tok-123","email":"test@example.com"}`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("FetchAccountDetail failed: %v", err)
	}
	if !HasGeminiGroup(detail.Groups) {
		t.Fatalf("expected Gemini Models group in merged groups, got %+v", detail.Groups)
	}
	if !HasClaudeGroup(detail.Groups) {
		t.Fatalf("expected Claude and GPT models group in merged groups, got %+v", detail.Groups)
	}
	if len(detail.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(detail.Groups))
	}
	if detail.Groups[0].DisplayName != "Gemini Models" {
		t.Fatalf("expected Gemini Models sorted first, got %q", detail.Groups[0].DisplayName)
	}
	if detail.Subscription == nil || detail.Subscription.Plan != "Pro" {
		t.Fatalf("expected Pro subscription, got %+v", detail.Subscription)
	}
}

func TestFetchQuotaSummaryMergesIntoApiCallShape(t *testing.T) {
	fh := splitEndpointHost()
	// Production only returns Gemini here, so the daily Claude/GPT group must be merged in.
	fh.responses[QuotaURLProd] = abi.HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Date": []string{"Fri, 23 Oct 2026 06:00:00 GMT"}, "X-Other": []string{"drop"}},
		Body:       []byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"window":"5h","remainingFraction":0.5}]}],"extra":"kept"}`),
	}
	fh.auths = map[string]abi.HostAuthGetResponse{
		"idx-1": {AuthIndex: "idx-1", Name: "ag.json", JSON: json.RawMessage(`{"access_token":"tok-123"}`)},
	}

	svc := NewService(fh, config.Default())
	resp, err := svc.FetchQuotaSummary(context.Background(), "idx-1", "proj-test-123")
	if err != nil {
		t.Fatalf("FetchQuotaSummary failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header["Date"]; len(got) != 1 {
		t.Fatalf("expected Date header passthrough, got %+v", resp.Header)
	}
	if _, ok := resp.Header["X-Other"]; ok {
		t.Fatal("unexpected passthrough of unrelated header")
	}
	var payload struct {
		Groups []struct {
			DisplayName string `json:"displayName"`
		} `json:"groups"`
		Extra string `json:"extra"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if len(payload.Groups) != 2 || payload.Groups[0].DisplayName != "Gemini Models" || payload.Groups[1].DisplayName != "Claude and GPT models" {
		t.Fatalf("unexpected merged groups: %+v", payload.Groups)
	}
	if payload.Extra != "kept" {
		t.Fatal("expected base payload fields to be preserved")
	}
}

func TestFetchQuotaSummarySurfacesUpstreamError(t *testing.T) {
	fh := &fakeHost{
		responses: map[string]abi.HTTPResponse{
			QuotaURLProd:         {StatusCode: http.StatusForbidden, Body: []byte(`{"error":{"message":"denied"}}`)},
			QuotaURLDaily:        {StatusCode: http.StatusForbidden, Body: []byte(`{"error":{"message":"denied"}}`)},
			QuotaURLDailySandbox: {StatusCode: http.StatusForbidden, Body: []byte(`{"error":{"message":"denied"}}`)},
		},
		auths: map[string]abi.HostAuthGetResponse{
			"idx-1": {AuthIndex: "idx-1", JSON: json.RawMessage(`{"access_token":"tok-123"}`)},
		},
	}
	svc := NewService(fh, config.Default())
	resp, err := svc.FetchQuotaSummary(context.Background(), "idx-1", "proj")
	if err != nil {
		t.Fatalf("FetchQuotaSummary failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(resp.Body, "denied") {
		t.Fatalf("expected upstream 403 passthrough, got %+v", resp)
	}

	if _, err := svc.FetchQuotaSummary(context.Background(), "missing", "proj"); err == nil {
		t.Fatal("expected error for credential without tokens")
	}
}

func splitEndpointHost() *fakeHost {
	return &fakeHost{
		responses: map[string]abi.HTTPResponse{
			LoadCodeAssistURLDaily: {
				StatusCode: http.StatusOK,
				Body:       []byte(`{"cloudaicompanionProject":"proj-test-123","paidTier":{"id":"g1-pro-tier","name":"Google One AI Pro"}}`),
			},
			// Simulate daily-cloudcode-pa returning ONLY Claude and GPT models (the linux.do/t/topic/3030611 bug)
			QuotaURLDaily: {
				StatusCode: http.StatusOK,
				Body:       []byte(`{"groups":[{"displayName":"Claude and GPT models","buckets":[{"window":"5h","displayName":"Five Hour Limit Remaining","remainingFraction":1,"resetTime":"2026-10-23T06:00:00Z"},{"window":"weekly","displayName":"Weekly Limit Remaining","remainingFraction":0.89,"resetTime":"2026-10-25T06:00:00Z"}]}]}`),
			},
			QuotaURLDailySandbox: {
				StatusCode: http.StatusOK,
				Body:       []byte(`{"groups":[{"displayName":"Claude and GPT models","buckets":[{"window":"5h","displayName":"Five Hour Limit Remaining","remainingFraction":1}]}]}`),
			},
			// Production endpoint returns both Gemini Models and Claude and GPT models
			QuotaURLProd: {
				StatusCode: http.StatusOK,
				Body:       []byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"window":"5h","displayName":"Five Hour Limit Remaining","remainingFraction":0.9997,"resetTime":"2026-10-23T05:00:00Z"},{"window":"weekly","displayName":"Weekly Limit Remaining","remainingFraction":0.9588,"resetTime":"2026-10-29T05:00:00Z"}]},{"displayName":"Claude and GPT models","buckets":[{"window":"5h","displayName":"Five Hour Limit Remaining","remainingFraction":1,"resetTime":"2026-10-23T06:00:00Z"},{"window":"weekly","displayName":"Weekly Limit Remaining","remainingFraction":0.89,"resetTime":"2026-10-25T06:00:00Z"}]}]}`),
			},
		},
	}
}
