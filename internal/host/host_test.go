package host

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
)

// CPA's host.http.do result is an untagged Go struct, so it arrives as
// {"StatusCode":200,"Headers":{...},"Body":"<base64>"}.
func TestHTTPDoDecodesHostResponse(t *testing.T) {
	body := `{"groups":[{"displayName":"Gemini Models"}]}`
	result := `{"StatusCode":200,"Headers":{"Content-Type":["application/json"]},"Body":"` +
		base64.StdEncoding.EncodeToString([]byte(body)) + `"}`

	r := RPC{Call: func(method string, _ []byte) ([]byte, error) {
		switch method {
		case abi.MethodHostHTTPOperationOpen:
			return []byte(`{"ok":true,"result":{"operation_id":"op-1"}}`), nil
		case abi.MethodHostHTTPDo:
			return []byte(`{"ok":true,"result":` + result + `}`), nil
		}
		return []byte(`{"ok":true}`), nil
	}}

	resp, err := r.HTTPDo(context.Background(), abi.HTTPRequest{Method: "POST", URL: "https://example.invalid"}, time.Second)
	if err != nil {
		t.Fatalf("HTTPDo: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != body {
		t.Fatalf("Body = %q, want %q", resp.Body, body)
	}
	if got := resp.Headers.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}
