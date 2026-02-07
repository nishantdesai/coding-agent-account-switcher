package ags

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func inspectAuth(tool Tool, raw []byte) AuthInsight {
	switch tool {
	case ToolCodex:
		return inspectCodex(raw)
	case ToolPi:
		return inspectPi(raw)
	case ToolClaude:
		return inspectClaude(raw)
	default:
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
		}
	}
}

func inspectCodex(raw []byte) AuthInsight {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"invalid JSON"},
		}
	}

	insight := AuthInsight{
		Status:       "unknown",
		NeedsRefresh: "unknown",
	}

	lastRefreshRaw, ok := payload["last_refresh"].(string)
	if ok && strings.TrimSpace(lastRefreshRaw) != "" {
		insight.LastRefresh = lastRefreshRaw
	}

	tokens, ok := payload["tokens"].(map[string]any)
	if !ok {
		insight.Details = append(insight.Details, "tokens object missing")
		return insight
	}

	accessToken, ok := tokens["access_token"].(string)
	if !ok || accessToken == "" {
		insight.Details = append(insight.Details, "access_token missing")
		return insight
	}

	expiry, ok := extractJWTExpiry(accessToken)
	if !ok {
		insight.Details = append(insight.Details, "could not parse JWT exp from access_token")
		return insight
	}

	insight.ExpiresAt = expiry.Format(time.RFC3339)
	status := classifyExpiry(expiry)
	insight.Status = status
	insight.NeedsRefresh = needsRefreshFromStatus(status)
	return insight
}

func inspectPi(raw []byte) AuthInsight {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"invalid JSON"},
		}
	}

	type providerStatus struct {
		name      string
		status    string
		expiresAt time.Time
	}
	statuses := []providerStatus{}

	for key, value := range payload {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		expRaw, ok := entry["expires"]
		if !ok {
			continue
		}
		expMillis, ok := numberToFloat(expRaw)
		if !ok {
			continue
		}
		expiry := time.UnixMilli(int64(expMillis)).UTC()
		statuses = append(statuses, providerStatus{
			name:      key,
			status:    classifyExpiry(expiry),
			expiresAt: expiry,
		})
	}

	if len(statuses) == 0 {
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"no provider expires fields found"},
		}
	}

	worst := statuses[0]
	for _, s := range statuses[1:] {
		if statusRank(s.status) > statusRank(worst.status) {
			worst = s
		}
	}

	details := make([]string, 0, len(statuses))
	for _, s := range statuses {
		details = append(details, fmt.Sprintf("%s=%s (%s)", s.name, s.status, s.expiresAt.Format(time.RFC3339)))
	}

	return AuthInsight{
		Status:       worst.status,
		ExpiresAt:    worst.expiresAt.Format(time.RFC3339),
		NeedsRefresh: needsRefreshFromStatus(worst.status),
		Details:      details,
	}
}

func inspectClaude(raw []byte) AuthInsight {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"invalid JSON"},
		}
	}

	_, hasOAuthAccount := payload["oauthAccount"]
	if hasOAuthAccount {
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"oauthAccount present, but token expiry is not available in this file format"},
		}
	}

	return AuthInsight{
		Status:       "unknown",
		NeedsRefresh: "unknown",
		Details:      []string{"no known expiry fields found"},
	}
}

func extractJWTExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payloadSegment := parts[1]
	padding := len(payloadSegment) % 4
	if padding > 0 {
		payloadSegment += strings.Repeat("=", 4-padding)
	}

	raw, err := base64.URLEncoding.DecodeString(payloadSegment)
	if err != nil {
		return time.Time{}, false
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return time.Time{}, false
	}

	expRaw, ok := claims["exp"]
	if !ok {
		return time.Time{}, false
	}
	exp, ok := numberToFloat(expRaw)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0).UTC(), true
}

func numberToFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func classifyExpiry(expiry time.Time) string {
	d := time.Until(expiry)
	if d <= 0 {
		return "expired"
	}
	if d <= 15*time.Minute {
		return "expiring_soon"
	}
	return "valid"
}

func needsRefreshFromStatus(status string) string {
	switch status {
	case "expired", "expiring_soon":
		return "yes"
	case "valid":
		return "no"
	default:
		return "unknown"
	}
}

func statusRank(status string) int {
	switch status {
	case "expired":
		return 3
	case "expiring_soon":
		return 2
	case "valid":
		return 1
	default:
		return 0
	}
}
