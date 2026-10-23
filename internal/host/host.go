// Package host wraps CLIProxyAPI host callbacks over the native C-ABI.
package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azurefly/antigravity-category-plugin/internal/abi"
	"github.com/Azurefly/antigravity-category-plugin/internal/config"
)

// Caller sends one host callback method with a JSON payload and returns the raw JSON envelope.
type Caller func(method string, payload []byte) ([]byte, error)

// Host provides access to CPA host capabilities (credentials, outbound HTTP, logging).
type Host interface {
	AuthList() ([]abi.HostAuthFileEntry, error)
	AuthGet(authIndex string) (abi.HostAuthGetResponse, error)
	AuthSave(name string, rawJSON json.RawMessage) (abi.HostAuthSaveResponse, error)
	HTTPDo(ctx context.Context, req abi.HTTPRequest, timeout time.Duration) (abi.HTTPResponse, error)
	Log(level, message string, fields map[string]any)
}

// RPC implements Host on top of a native C-ABI Caller with net/http fallback.
type RPC struct {
	Call       Caller
	HTTPClient *http.Client
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		HTTPStatus int    `json:"http_status,omitempty"`
	} `json:"error,omitempty"`
}

func (r RPC) call(method string, payload any, out any) error {
	if r.Call == nil {
		return fmt.Errorf("host callback %s unavailable", method)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := r.Call(method, raw)
	if err != nil {
		return err
	}
	if len(resp) == 0 {
		return fmt.Errorf("host callback %s returned empty response", method)
	}
	var env envelope
	if err := json.Unmarshal(resp, &env); err != nil {
		return fmt.Errorf("decode host envelope for %s: %w", method, err)
	}
	if !env.OK {
		if env.Error != nil {
			return fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return fmt.Errorf("host callback %s failed", method)
	}
	if out == nil || len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

func (r RPC) AuthList() ([]abi.HostAuthFileEntry, error) {
	var resp struct {
		Files []abi.HostAuthFileEntry `json:"files"`
	}
	if err := r.call(abi.MethodHostAuthList, map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return resp.Files, nil
}

func (r RPC) AuthGet(authIndex string) (abi.HostAuthGetResponse, error) {
	var resp abi.HostAuthGetResponse
	err := r.call(abi.MethodHostAuthGet, abi.HostAuthGetRequest{AuthIndex: authIndex}, &resp)
	return resp, err
}

func (r RPC) AuthSave(name string, rawJSON json.RawMessage) (abi.HostAuthSaveResponse, error) {
	var resp abi.HostAuthSaveResponse
	err := r.call(abi.MethodHostAuthSave, abi.HostAuthSaveRequest{Name: name, JSON: rawJSON}, &resp)
	return resp, err
}

type callbackIDKey struct{}

// WithCallbackID attaches the host_callback_id from an incoming RPC request to ctx.
func WithCallbackID(ctx context.Context, callbackID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	callbackID = strings.TrimSpace(callbackID)
	if callbackID == "" {
		return ctx
	}
	return context.WithValue(ctx, callbackIDKey{}, callbackID)
}

func callbackIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(callbackIDKey{}).(string)
	return strings.TrimSpace(v)
}

type hostHTTPRequest struct {
	HostCallbackID string      `json:"host_callback_id,omitempty"`
	OperationID    string      `json:"operation_id,omitempty"`
	Method         string      `json:"method,omitempty"`
	URL            string      `json:"url,omitempty"`
	Headers        http.Header `json:"headers,omitempty"`
	Body           []byte      `json:"body,omitempty"`
}

func (r RPC) HTTPDo(ctx context.Context, req abi.HTTPRequest, timeout time.Duration) (abi.HTTPResponse, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Never outlive the caller's deadline, and don't start new attempts once it has passed.
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if err := ctx.Err(); err != nil {
		return abi.HTTPResponse{}, err
	}
	if timeout <= 0 {
		return abi.HTTPResponse{}, context.DeadlineExceeded
	}
	if r.Call != nil {
		cbID := callbackIDFromContext(ctx)
		var opened struct {
			OperationID string `json:"operation_id"`
		}
		openReq := map[string]any{}
		if cbID != "" {
			openReq["host_callback_id"] = cbID
		}
		if err := r.call(abi.MethodHostHTTPOperationOpen, openReq, &opened); err == nil {
			opID := strings.TrimSpace(opened.OperationID)
			if opID != "" {
				done := make(chan struct{})
				defer close(done)
				go func() {
					timer := time.NewTimer(timeout)
					defer timer.Stop()
					select {
					case <-done:
						return
					case <-timer.C:
					case <-ctx.Done():
					}
					cancelReq := map[string]any{"operation_id": opID}
					if cbID != "" {
						cancelReq["host_callback_id"] = cbID
					}
					_ = r.call(abi.MethodHostHTTPCancel, cancelReq, nil)
				}()
				var resp abi.HTTPResponse
				err := r.call(abi.MethodHostHTTPDo, hostHTTPRequest{
					HostCallbackID: cbID,
					OperationID:    opID,
					Method:         req.Method,
					URL:            req.URL,
					Headers:        req.Headers,
					Body:           req.Body,
				}, &resp)
				if err == nil {
					return resp, nil
				}
				// The cancellable attempt already used up the budget; retrying would
				// multiply the wait the Management Center sits on.
				if ctxErr := ctx.Err(); ctxErr != nil {
					return abi.HTTPResponse{}, ctxErr
				}
				return abi.HTTPResponse{}, err
			}
		}
		var resp abi.HTTPResponse
		if err := r.call(abi.MethodHostHTTPDo, hostHTTPRequest{
			HostCallbackID: cbID,
			Method:         req.Method,
			URL:            req.URL,
			Headers:        req.Headers,
			Body:           req.Body,
		}, &resp); err == nil {
			return resp, nil
		}
	}

	// Fallback to standard net/http client when outside a host callback context.
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctxWithTimeout, req.Method, req.URL, bodyReader)
	if err != nil {
		return abi.HTTPResponse{}, err
	}
	for k, vals := range req.Headers {
		for _, v := range vals {
			httpReq.Header.Add(k, v)
		}
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return abi.HTTPResponse{}, err
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 4<<20))
	if err != nil {
		return abi.HTTPResponse{}, err
	}
	return abi.HTTPResponse{
		StatusCode: httpResp.StatusCode,
		Headers:    httpResp.Header,
		Body:       respBody,
	}, nil
}

func (r RPC) Log(level, message string, fields map[string]any) {
	if r.Call == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["plugin"] = config.PluginID
	_ = r.call(abi.MethodHostLog, map[string]any{
		"level":   level,
		"message": message,
		"fields":  fields,
	}, nil)
}
