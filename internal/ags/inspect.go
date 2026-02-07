package ags

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
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

	tokenInfo := inspectAccessToken(accessToken)
	if tokenInfo.IsJWT {
		insight.Details = append(insight.Details, describeJWTToken("access_token", tokenInfo))
	} else {
		insight.Details = append(insight.Details, "access_token format=opaque (not JWT)")
	}

	if !tokenInfo.HasExp {
		insight.Details = append(insight.Details, "could not parse JWT exp from access_token")
		return insight
	}

	insight.ExpiresAt = tokenInfo.ExpiresAt.Format(time.RFC3339)
	status := classifyExpiry(tokenInfo.ExpiresAt)
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
	tokenDetails := []string{}

	for key, value := range payload {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}

		if accessToken, ok := entry["access"].(string); ok && strings.TrimSpace(accessToken) != "" {
			tokenInfo := inspectAccessToken(accessToken)
			if tokenInfo.IsJWT {
				tokenDetails = append(tokenDetails, describeJWTToken(key+".access", tokenInfo))
			} else {
				tokenDetails = append(tokenDetails, fmt.Sprintf("%s.access format=opaque (not JWT)", key))
			}
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

	sort.Strings(tokenDetails)
	if len(statuses) == 0 {
		details := []string{"no provider expires fields found"}
		details = append(details, tokenDetails...)
		return AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      details,
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statusRank(statuses[i].status) > statusRank(statuses[j].status)
	})
	worst := statuses[0]

	details := make([]string, 0, len(statuses)+len(tokenDetails))
	for _, s := range statuses {
		details = append(details, fmt.Sprintf("%s=%s (%s)", s.name, s.status, s.expiresAt.Format(time.RFC3339)))
	}
	details = append(details, tokenDetails...)

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

type accessTokenInsight struct {
	IsJWT     bool
	HeaderAlg string
	ClaimKeys []string
	HasExp    bool
	ExpiresAt time.Time
}

func inspectAccessToken(token string) accessTokenInsight {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return accessTokenInsight{}
	}

	headerRaw, err := decodeJWTSegment(parts[0])
	if err != nil {
		return accessTokenInsight{}
	}
	claimsRaw, err := decodeJWTSegment(parts[1])
	if err != nil {
		return accessTokenInsight{}
	}

	var header map[string]any
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return accessTokenInsight{}
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return accessTokenInsight{}
	}

	info := accessTokenInsight{IsJWT: true}
	if alg, ok := header["alg"].(string); ok && strings.TrimSpace(alg) != "" {
		info.HeaderAlg = alg
	}
	if len(claims) > 0 {
		info.ClaimKeys = make([]string, 0, len(claims))
		for key := range claims {
			info.ClaimKeys = append(info.ClaimKeys, key)
		}
		sort.Strings(info.ClaimKeys)
	}

	if expRaw, ok := claims["exp"]; ok {
		if exp, ok := numberToFloat(expRaw); ok {
			info.HasExp = true
			info.ExpiresAt = time.Unix(int64(exp), 0).UTC()
		}
	}
	return info
}

func describeJWTToken(tokenName string, info accessTokenInsight) string {
	claims := "-"
	if len(info.ClaimKeys) > 0 {
		claims = strings.Join(info.ClaimKeys, ",")
	}
	if info.HeaderAlg == "" {
		return fmt.Sprintf("%s format=jwt claims=%s", tokenName, claims)
	}
	return fmt.Sprintf("%s format=jwt alg=%s claims=%s", tokenName, info.HeaderAlg, claims)
}

func extractJWTExpiry(token string) (time.Time, bool) {
	info := inspectAccessToken(token)
	if !info.HasExp {
		return time.Time{}, false
	}
	return info.ExpiresAt, true
}

func decodeJWTSegment(segment string) ([]byte, error) {
	padding := len(segment) % 4
	if padding > 0 {
		segment += strings.Repeat("=", 4-padding)
	}
	return base64.URLEncoding.DecodeString(segment)
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
