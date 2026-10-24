// Package abi defines the CLIProxyAPI C-ABI and JSON RPC contract types.
package abi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

const (
	// ABIVersion tracks the native C ABI shape exported by the shared library.
	ABIVersion uint32 = 1
	// SchemaVersion tracks the RPC JSON contract exchanged at plugin.register.
	SchemaVersion uint32 = 6
)

const (
	MethodPluginRegister    = "plugin.register"
	MethodPluginQuiesce     = "plugin.quiesce"
	MethodPluginReconfigure = "plugin.reconfigure"
	MethodPluginShutdown    = "plugin.shutdown"

	MethodManagementRegister = "management.register"
	MethodManagementHandle   = "management.handle"

	MethodQuotaIdentifier = "quota.identifier"
	MethodQuotaDescribe   = "quota.describe"
	MethodQuotaFetch      = "quota.fetch"
	MethodQuotaReset      = "quota.reset"

	MethodHostHTTPDo            = "host.http.do"
	MethodHostHTTPOperationOpen = "host.http.operation_open"
	MethodHostHTTPCancel        = "host.http.cancel"
	MethodHostLog               = "host.log"
	MethodHostAuthList          = "host.auth.list"
	MethodHostAuthGet           = "host.auth.get"
	MethodHostAuthSave          = "host.auth.save"
)

type ConfigFieldType string

const (
	ConfigFieldTypeString  ConfigFieldType = "string"
	ConfigFieldTypeNumber  ConfigFieldType = "number"
	ConfigFieldTypeInteger ConfigFieldType = "integer"
	ConfigFieldTypeBoolean ConfigFieldType = "boolean"
	ConfigFieldTypeEnum    ConfigFieldType = "enum"
	ConfigFieldTypeArray   ConfigFieldType = "array"
	ConfigFieldTypeObject  ConfigFieldType = "object"
)

type ConfigField struct {
	Name        string          `json:"Name"`
	Type        ConfigFieldType `json:"Type"`
	Default     any             `json:"Default,omitempty"`
	EnumValues  []string        `json:"EnumValues,omitempty"`
	Description string          `json:"Description"`
}

type Metadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Logo             string        `json:"Logo,omitempty"`
	ConfigFields     []ConfigField `json:"ConfigFields"`
}

type Registration struct {
	SchemaVersion uint32          `json:"schema_version"`
	Metadata      Metadata        `json:"metadata"`
	Capabilities  map[string]bool `json:"capabilities"`
}

type LifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type IdentifierResponse struct {
	Identifier string `json:"identifier"`
}

type QuotaDescribeRequest struct {
	Plugin Metadata `json:"plugin,omitempty"`
}

type QuotaDescribeResponse struct {
	SupportedProviders []string `json:"supported_providers,omitempty"`
	DisplayName        string   `json:"display_name,omitempty"`
	SupportsReset      bool     `json:"supports_reset,omitempty"`
}

type QuotaFetchRequest struct {
	AuthIndex      string            `json:"auth_index"`
	AuthID         string            `json:"auth_id"`
	Provider       string            `json:"provider"`
	StorageJSON    []byte            `json:"storage_json,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	HostCallbackID string            `json:"host_callback_id,omitempty"`
}

type QuotaSubscription struct {
	Plan     string `json:"plan,omitempty"`
	TierName string `json:"tierName,omitempty"`
	TierID   string `json:"tierId,omitempty"`
}

type QuotaBucket struct {
	Window            string  `json:"window,omitempty"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime,omitempty"`
	Description       string  `json:"description,omitempty"`
}

type QuotaGroup struct {
	DisplayName string        `json:"displayName,omitempty"`
	Description string        `json:"description,omitempty"`
	Buckets     []QuotaBucket `json:"buckets,omitempty"`
}

type QuotaMetric struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit,omitempty"`
	Format   string  `json:"format,omitempty"`
	Currency string  `json:"currency,omitempty"`
}

type QuotaFetchResponse struct {
	Subscription       *QuotaSubscription `json:"subscription,omitempty"`
	Summary            []QuotaMetric      `json:"summary,omitempty"`
	ServerTimeOffsetMs int64              `json:"serverTimeOffsetMs,omitempty"`
	Groups             []QuotaGroup       `json:"groups,omitempty"`
}

type QuotaResetRequest struct {
	AuthIndex      string            `json:"auth_index"`
	AuthID         string            `json:"auth_id"`
	Provider       string            `json:"provider"`
	StorageJSON    []byte            `json:"storage_json,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	HostCallbackID string            `json:"host_callback_id,omitempty"`
}

type QuotaResetResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type ManagementRoute struct {
	Method      string `json:"Method"`
	Path        string `json:"Path"`
	Description string `json:"Description"`
}

type ResourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type ManagementRegistrationResponse struct {
	Routes    []ManagementRoute `json:"routes,omitempty"`
	Resources []ResourceRoute   `json:"resources,omitempty"`
}

type ManagementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type ManagementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type HTTPRequest struct {
	Method  string      `json:"method,omitempty"`
	URL     string      `json:"url,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Body    []byte      `json:"body,omitempty"`
}

// HTTPResponse mirrors CPA's untagged pluginapi.HTTPResponse, which host.http.do
// serializes with Go field names ("StatusCode", not "status_code").
type HTTPResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type HostAuthFileEntry struct {
	ID          string    `json:"id,omitempty"`
	AuthIndex   string    `json:"auth_index,omitempty"`
	Name        string    `json:"name"`
	Type        string    `json:"type,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	Label       string    `json:"label,omitempty"`
	Status      string    `json:"status,omitempty"`
	Disabled    bool      `json:"disabled,omitempty"`
	Unavailable bool      `json:"unavailable,omitempty"`
	Email       string    `json:"email,omitempty"`
	ProjectID   string    `json:"project_id,omitempty"`
	AccountType string    `json:"account_type,omitempty"`
	Account     string    `json:"account,omitempty"`
	Priority    int       `json:"priority,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type HostAuthGetRequest struct {
	AuthIndex string `json:"auth_index"`
}

type HostAuthGetResponse struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name,omitempty"`
	Path      string          `json:"path,omitempty"`
	JSON      json.RawMessage `json:"json"`
}

type HostAuthSaveRequest struct {
	Name string          `json:"name"`
	JSON json.RawMessage `json:"json"`
}

type HostAuthSaveResponse struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
