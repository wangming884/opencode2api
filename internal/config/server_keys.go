package config

import (
	"strings"
	"time"
)

// ServerKeyMetadata holds the WebUI-managed details of one server key: its
// display name, whether it is disabled, when it was created, and the optional
// traffic policy enforced for it. It is keyed by the existing key fingerprint;
// secret values remain in server_keys for compatibility with existing
// configuration and clients.
type ServerKeyMetadata struct {
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	// ExpiresAt is when the key stops authenticating. Absent means it never
	// expires. The key is kept in the configuration so it can be extended
	// instead of being reissued.
	ExpiresAt *time.Time       `json:"expires_at,omitempty"`
	Limits    *ServerKeyLimits `json:"limits,omitempty"`
}

// ServerKeyLimits is the per-key traffic policy. Every field is optional and
// zero means unlimited, so an empty object is the same as no policy at all.
//
// RPM and DailyRequests are checked before the request is routed, so they cap
// traffic without reaching the upstream. DailyTokens is a budget rather than a
// cap: usage is only known after the upstream reports it, so the request that
// crosses the budget still completes and the next one is refused.
type ServerKeyLimits struct {
	// RPM is the request ceiling in any rolling minute.
	RPM int `json:"rpm,omitempty"`
	// DailyRequests is the request ceiling per UTC day.
	DailyRequests int `json:"daily_requests,omitempty"`
	// DailyTokens is the token budget per UTC day.
	DailyTokens int64 `json:"daily_tokens,omitempty"`
	// Models restricts which model IDs the key may ask for. A trailing "*"
	// matches a prefix ("gpt-*"); an empty list allows every model.
	Models []string `json:"models,omitempty"`
}

// IsZero reports whether the policy limits nothing, which lets the metadata
// drop an empty object instead of storing one.
func (l ServerKeyLimits) IsZero() bool {
	return l.RPM <= 0 && l.DailyRequests <= 0 && l.DailyTokens <= 0 && len(l.Models) == 0
}

// ServerKeyLimitsOf returns the policy of the key fingerprint id. A key
// without metadata, or with an empty policy, is unlimited.
func (c Config) ServerKeyLimitsOf(id string) ServerKeyLimits {
	if meta, ok := c.ServerKeyMetadata[id]; ok && meta.Limits != nil {
		return *meta.Limits
	}
	return ServerKeyLimits{}
}

// ServerKeyAllowsModel reports whether the key fingerprint id may request
// model. A policy with no model list allows everything. A pattern ending in "*"
// matches by prefix, so "gpt-*" admits every gpt model.
func (c Config) ServerKeyAllowsModel(id, model string) bool {
	patterns := c.ServerKeyLimitsOf(id).Models
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(model, prefix) {
				return true
			}
			continue
		}
		if pattern == model {
			return true
		}
	}
	return false
}

// ServerKeyAllowsRequest reports whether the key may ask for the public name
// requested, which may resolve to served. Either name in the policy admits the
// request: an operator writing "gpt-*" means the public name, while one writing
// the real model ID means the model itself, and both are reasonable.
func (c Config) ServerKeyAllowsRequest(id, requested, served string) bool {
	if c.ServerKeyAllowsModel(id, requested) {
		return true
	}
	return served != requested && c.ServerKeyAllowsModel(id, served)
}

// ServerKeyEnabled reports whether key may authenticate clients. A key
// without metadata is enabled.
func (c Config) ServerKeyEnabled(key string) bool {
	return !c.ServerKeyMetadata[Fingerprint(key)].Disabled
}

// ServerKeyExpired reports whether the key fingerprint id is past its expiry.
// Expiry is separate from Disabled so the console can tell a key that was
// switched off from one whose time ran out.
func (c Config) ServerKeyExpired(id string) bool {
	meta, ok := c.ServerKeyMetadata[id]
	if !ok || meta.ExpiresAt == nil {
		return false
	}
	return !time.Now().Before(*meta.ExpiresAt)
}

// FirstEnabledServerKey returns the first enabled server key, or "" when every
// key is disabled. The Playground sends its requests with this key.
func (c Config) FirstEnabledServerKey() string {
	for _, key := range c.ServerKeys {
		if c.ServerKeyEnabled(key) {
			return key
		}
	}
	return ""
}
