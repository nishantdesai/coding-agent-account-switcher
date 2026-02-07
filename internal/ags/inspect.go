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

	insight.AccountID = extractStringClaim(tokens, "account_id")

	idToken := extractStringClaim(tokens, "id_token")
	if idToken != "" {
		idInfo := inspectAccessToken(idToken)
		if idInfo.IsJWT {
			if email := resolveCodexEmailFromJWT(idInfo); email != "" {
				insight.AccountEmail = email
			}
			if plan := resolveCodexPlanFromJWT(idInfo); plan != "" {
				insight.AccountPlan = normalizePlan(plan)
			}
			if insight.AccountID == "" {
				if jwtAccountID := resolveCodexAccountIDFromJWT(idInfo); jwtAccountID != "" {
					insight.AccountID = jwtAccountID
				}
			}
		}
	}

	accessToken, ok := tokens["access_token"].(string)
	if !ok || accessToken == "" {
		insight.Details = append(insight.Details, "access_token missing")
		return insight
	}

	tokenInfo := inspectAccessToken(accessToken)
	if !tokenInfo.HasExp {
		insight.Details = append(insight.Details, "could not parse access_token exp")
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

	sort.Slice(statuses, func(i, j int) bool {
		return statusRank(statuses[i].status) > statusRank(statuses[j].status)
	})
	worst := statuses[0]

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

type accessTokenInsight struct {
	IsJWT     bool
	HeaderAlg string
	ClaimKeys []string
	Claims    map[string]any
	HasExp    bool
	ExpiresAt time.Time
	HasIat    bool
	IssuedAt  time.Time
	Issuer    string
	Subject   string
	Audience  string
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

	info := accessTokenInsight{IsJWT: true, Claims: claims}
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
	if iatRaw, ok := claims["iat"]; ok {
		if iat, ok := numberToFloat(iatRaw); ok {
			info.HasIat = true
			info.IssuedAt = time.Unix(int64(iat), 0).UTC()
		}
	}

	info.Issuer = extractStringClaim(claims, "iss")
	info.Subject = extractStringClaim(claims, "sub")
	info.Audience = extractAudienceClaim(claims)

	return info
}

func resolveCodexEmailFromJWT(info accessTokenInsight) string {
	if !info.IsJWT {
		return ""
	}
	if email := extractStringClaim(info.Claims, "email"); email != "" {
		return email
	}
	if profile, ok := info.Claims["https://api.openai.com/profile"].(map[string]any); ok {
		if email := extractStringClaim(profile, "email"); email != "" {
			return email
		}
	}
	if profile, ok := info.Claims["profile"].(map[string]any); ok {
		if email := extractStringClaim(profile, "email"); email != "" {
			return email
		}
	}
	return ""
}

func resolveCodexPlanFromJWT(info accessTokenInsight) string {
	if !info.IsJWT {
		return ""
	}
	if auth, ok := info.Claims["https://api.openai.com/auth"].(map[string]any); ok {
		if plan := extractStringClaim(auth, "chatgpt_plan_type"); plan != "" {
			return plan
		}
		if plan := extractStringClaim(auth, "plan"); plan != "" {
			return plan
		}
	}
	if auth, ok := info.Claims["auth"].(map[string]any); ok {
		if plan := extractStringClaim(auth, "chatgpt_plan_type"); plan != "" {
			return plan
		}
		if plan := extractStringClaim(auth, "plan"); plan != "" {
			return plan
		}
	}
	if plan := extractStringClaim(info.Claims, "chatgpt_plan_type"); plan != "" {
		return plan
	}
	if plan := extractStringClaim(info.Claims, "plan"); plan != "" {
		return plan
	}
	return ""
}

func resolveCodexAccountIDFromJWT(info accessTokenInsight) string {
	if !info.IsJWT {
		return ""
	}
	if auth, ok := info.Claims["https://api.openai.com/auth"].(map[string]any); ok {
		if accountID := extractStringClaim(auth, "account_id"); accountID != "" {
			return accountID
		}
		if accountID := extractStringClaim(auth, "accountId"); accountID != "" {
			return accountID
		}
		if accountID := extractStringClaim(auth, "chatgpt_account_id"); accountID != "" {
			return accountID
		}
	}
	if accountID := extractStringClaim(info.Claims, "account_id"); accountID != "" {
		return accountID
	}
	if accountID := extractStringClaim(info.Claims, "accountId"); accountID != "" {
		return accountID
	}
	return ""
}

func normalizePlan(plan string) string {
	cleaned := strings.TrimSpace(strings.ToLower(plan))
	switch cleaned {
	case "plus", "chatgpt_plus":
		return "Plus"
	case "pro", "chatgpt_pro":
		return "Pro"
	case "team", "chatgpt_team":
		return "Team"
	case "enterprise", "chatgpt_enterprise":
		return "Enterprise"
	case "free", "chatgpt_free":
		return "Free"
	default:
		if cleaned == "" {
			return ""
		}
		return strings.ToUpper(cleaned[:1]) + cleaned[1:]
	}
}

func extractStringClaim(claims map[string]any, key string) string {
	v, ok := claims[key].(string)
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return v
}

func extractAudienceClaim(claims map[string]any) string {
	v, ok := claims["aud"]
	if !ok {
		return ""
	}
	switch aud := v.(type) {
	case string:
		aud = strings.TrimSpace(aud)
		if aud == "" {
			return ""
		}
		return aud
	case []any:
		values := make([]string, 0, len(aud))
		for _, item := range aud {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			values = append(values, s)
		}
		if len(values) == 0 {
			return ""
		}
		return strings.Join(values, ",")
	default:
		return ""
	}
}

func describeJWTToken(tokenName string, info accessTokenInsight) string {
	parts := []string{fmt.Sprintf("%s format=jwt", tokenName)}
	if info.HeaderAlg != "" {
		parts = append(parts, fmt.Sprintf("alg=%s", info.HeaderAlg))
	}
	if info.HasExp {
		parts = append(parts, fmt.Sprintf("exp=%s", info.ExpiresAt.Format(time.RFC3339)))
	}
	if info.HasIat {
		parts = append(parts, fmt.Sprintf("iat=%s", info.IssuedAt.Format(time.RFC3339)))
	}
	if info.Issuer != "" {
		parts = append(parts, fmt.Sprintf("iss=%s", info.Issuer))
	}
	if info.Subject != "" {
		parts = append(parts, fmt.Sprintf("sub=%s", info.Subject))
	}
	if info.Audience != "" {
		parts = append(parts, fmt.Sprintf("aud=%s", info.Audience))
	}
	claims := "-"
	if len(info.ClaimKeys) > 0 {
		claims = strings.Join(info.ClaimKeys, ",")
	}
	parts = append(parts, fmt.Sprintf("claims=%s", claims))
	return strings.Join(parts, " ")
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
