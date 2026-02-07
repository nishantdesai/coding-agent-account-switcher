package ags

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func jwtWithExp(t *testing.T, exp any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claimsBytes, err := json.Marshal(map[string]any{"exp": exp})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	claims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	return header + "." + claims + ".sig"
}

func TestInspectAuthDispatch(t *testing.T) {
	codexRaw := []byte(`{"tokens":{"access_token":"` + jwtWithExp(t, time.Now().Add(time.Hour).Unix()) + `"}}`)
	if got := inspectAuth(ToolCodex, codexRaw); got.Status == "unknown" {
		t.Fatalf("expected codex dispatch")
	}

	piRaw := []byte(`{"provider":{"expires":` + strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10) + `}}`)
	if got := inspectAuth(ToolPi, piRaw); got.Status == "unknown" {
		t.Fatalf("expected pi dispatch")
	}

	got := inspectAuth(Tool("unknown"), []byte(`{}`))
	if got.Status != "unknown" || got.NeedsRefresh != "unknown" {
		t.Fatalf("unexpected fallback insight: %+v", got)
	}
}

func TestInspectCodexBranches(t *testing.T) {
	if got := inspectCodex([]byte("not-json")); len(got.Details) == 0 || got.Details[0] != "invalid JSON" {
		t.Fatalf("invalid json branch not hit: %+v", got)
	}

	if got := inspectCodex([]byte(`{"x":1}`)); len(got.Details) == 0 || got.Details[0] != "tokens object missing" {
		t.Fatalf("missing tokens branch not hit: %+v", got)
	}

	if got := inspectCodex([]byte(`{"tokens":{}}`)); len(got.Details) == 0 || got.Details[0] != "access_token missing" {
		t.Fatalf("missing access token branch not hit: %+v", got)
	}

	got := inspectCodex([]byte(`{"tokens":{"access_token":"bad"}}`))
	joined := strings.Join(got.Details, " ")
	if !strings.Contains(joined, "could not parse access_token exp") {
		t.Fatalf("bad token branch not hit: %+v", got)
	}

	future := time.Now().UTC().Add(1 * time.Hour).Unix()
	validRaw := `{"last_refresh":"2026-01-01T00:00:00Z","tokens":{"access_token":"` + jwtWithExp(t, future) + `"}}`
	got = inspectCodex([]byte(validRaw))
	if got.Status != "valid" || got.NeedsRefresh != "no" || got.ExpiresAt == "" {
		t.Fatalf("valid branch failed: %+v", got)
	}
	if got.LastRefresh != "2026-01-01T00:00:00Z" {
		t.Fatalf("expected last refresh from payload, got %+v", got)
	}

	expSoon := time.Now().UTC().Add(5 * time.Minute).Unix()
	got = inspectCodex([]byte(`{"tokens":{"access_token":"` + jwtWithExp(t, expSoon) + `"}}`))
	if got.Status != "expiring_soon" || got.NeedsRefresh != "yes" {
		t.Fatalf("expiring soon branch failed: %+v", got)
	}

	expired := time.Now().UTC().Add(-1 * time.Minute).Unix()
	got = inspectCodex([]byte(`{"tokens":{"access_token":"` + jwtWithExp(t, expired) + `"}}`))
	if got.Status != "expired" || got.NeedsRefresh != "yes" {
		t.Fatalf("expired branch failed: %+v", got)
	}

	jwtNoExpHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	jwtNoExpClaims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1"}`))
	jwtNoExp := jwtNoExpHeader + "." + jwtNoExpClaims + ".sig"
	got = inspectCodex([]byte(`{"tokens":{"access_token":"` + jwtNoExp + `"}}`))
	if !strings.Contains(strings.Join(got.Details, " "), "could not parse access_token exp") {
		t.Fatalf("expected jwt-without-exp parse failure detail branch, got %+v", got)
	}
}

func TestInspectPiBranches(t *testing.T) {
	if got := inspectPi([]byte("not-json")); len(got.Details) == 0 || got.Details[0] != "invalid JSON" {
		t.Fatalf("invalid json branch not hit: %+v", got)
	}

	if got := inspectPi([]byte(`{"provider":{},"other":"x","badexp":{"expires":"x"}}`)); len(got.Details) == 0 || got.Details[0] != "no provider expires fields found" {
		t.Fatalf("no expires branch not hit: %+v", got)
	}

	validMillis := time.Now().UTC().Add(2 * time.Hour).UnixMilli()
	expiredMillis := time.Now().UTC().Add(-2 * time.Hour).UnixMilli()
	raw := `{"provider_a":{"expires":` + strconv.FormatInt(validMillis, 10) + `},"provider_b":{"expires":` + strconv.FormatInt(expiredMillis, 10) + `}}`
	got := inspectPi([]byte(raw))
	if got.Status != "expired" || got.NeedsRefresh != "yes" {
		t.Fatalf("expected worst provider status to be expired: %+v", got)
	}
	if len(got.Details) != 2 {
		t.Fatalf("expected two provider details: %+v", got)
	}
	joined := strings.Join(got.Details, " ")
	if !strings.Contains(joined, "provider_b=expired") || !strings.Contains(joined, "provider_a=valid") {
		t.Fatalf("unexpected details: %+v", got.Details)
	}
}

func TestInspectPiTokenDetails(t *testing.T) {
	expMillis := time.Now().UTC().Add(time.Hour).UnixMilli()
	jwt := jwtWithExp(t, time.Now().UTC().Add(time.Hour).Unix())
	raw := `{"openai-codex":{"access":"` + jwt + `","expires":` + strconv.FormatInt(expMillis, 10) + `},"anthropic":{"access":"opaque-token","expires":` + strconv.FormatInt(expMillis, 10) + `}}`
	got := inspectPi([]byte(raw))
	joined := strings.Join(got.Details, " ")
	if !strings.Contains(joined, "openai-codex=valid") {
		t.Fatalf("expected openai-codex status detail, got %+v", got.Details)
	}
	if !strings.Contains(joined, "anthropic=valid") {
		t.Fatalf("expected anthropic status detail, got %+v", got.Details)
	}
}

func TestExtractJWTExpiryBranches(t *testing.T) {
	if _, ok := extractJWTExpiry("bad"); ok {
		t.Fatalf("expected invalid parts branch")
	}
	if _, ok := extractJWTExpiry("a.*.c"); ok {
		t.Fatalf("expected decode failure branch")
	}

	payload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	if _, ok := extractJWTExpiry("a." + payload + ".c"); ok {
		t.Fatalf("expected claims json failure branch")
	}

	claimsNoExp := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u"}`))
	if _, ok := extractJWTExpiry("a." + claimsNoExp + ".c"); ok {
		t.Fatalf("expected missing exp branch")
	}

	claimsBadExp := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":"x"}`))
	if _, ok := extractJWTExpiry("a." + claimsBadExp + ".c"); ok {
		t.Fatalf("expected bad exp branch")
	}

	claimsPadded := base64.URLEncoding.EncodeToString([]byte(`{"exp":123456}`))
	headerPadded := base64.URLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	if got, ok := extractJWTExpiry(headerPadded + "." + claimsPadded + ".c"); !ok || got.Unix() != 123456 {
		t.Fatalf("expected valid parse on already padded payload")
	}

	exp := time.Now().UTC().Add(30 * time.Minute).Unix()
	tok := jwtWithExp(t, exp)
	got, ok := extractJWTExpiry(tok)
	if !ok || got.Unix() != exp {
		t.Fatalf("expected valid exp parse, got %v, ok=%v", got, ok)
	}
}

func TestInspectAccessTokenHelpers(t *testing.T) {
	jwtToken := jwtWithExp(t, 123)
	info := inspectAccessToken(jwtToken)
	if !info.IsJWT || !info.HasExp || info.ExpiresAt.Unix() != 123 {
		t.Fatalf("expected jwt with exp insight, got %+v", info)
	}
	if len(info.ClaimKeys) == 0 || info.ClaimKeys[0] != "exp" {
		t.Fatalf("expected claim key extraction, got %+v", info.ClaimKeys)
	}

	if got := describeJWTToken("access_token", info); !strings.Contains(got, "format=jwt") || !strings.Contains(got, "claims=exp") {
		t.Fatalf("unexpected describeJWTToken output: %q", got)
	}

	headerNoAlg := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1"}`))
	info = inspectAccessToken(headerNoAlg + "." + claims + ".sig")
	if !info.IsJWT || info.HasExp {
		t.Fatalf("expected jwt without exp, got %+v", info)
	}
	if got := describeJWTToken("access_token", info); strings.Contains(got, "alg=") {
		t.Fatalf("did not expect alg in description: %q", got)
	}

	if info := inspectAccessToken("opaque-token"); info.IsJWT {
		t.Fatalf("expected non-jwt token")
	}

	if info := inspectAccessToken("*.e30.sig"); info.IsJWT {
		t.Fatalf("expected header decode failure path")
	}
	if info := inspectAccessToken(base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".e30.sig"); info.IsJWT {
		t.Fatalf("expected header json failure path")
	}
	validHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	if info := inspectAccessToken(validHeader + ".*.sig"); info.IsJWT {
		t.Fatalf("expected claims decode failure path")
	}
	if info := inspectAccessToken(validHeader + "." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".sig"); info.IsJWT {
		t.Fatalf("expected claims json failure path")
	}
	emptyClaims := base64.RawURLEncoding.EncodeToString([]byte(`{}`))
	if info := inspectAccessToken(validHeader + "." + emptyClaims + ".sig"); !info.IsJWT || len(info.ClaimKeys) != 0 {
		t.Fatalf("expected jwt with empty claim set, got %+v", info)
	}

	seg := base64.RawURLEncoding.EncodeToString([]byte(`{"x":1}`))
	if _, err := decodeJWTSegment(seg); err != nil {
		t.Fatalf("expected decode success with raw-url segment: %v", err)
	}
	if _, err := decodeJWTSegment("*"); err == nil {
		t.Fatalf("expected decode failure for invalid segment")
	}
}

func TestNumberToFloatAndStatusHelpers(t *testing.T) {
	cases := []any{float64(1.2), int(1), int32(2), int64(3), json.Number("4.5")}
	for _, c := range cases {
		if _, ok := numberToFloat(c); !ok {
			t.Fatalf("expected numeric conversion for %#v", c)
		}
	}
	if _, ok := numberToFloat(json.Number("x")); ok {
		t.Fatalf("expected bad json.Number failure")
	}
	if _, ok := numberToFloat(struct{}{}); ok {
		t.Fatalf("expected default failure")
	}

	if classifyExpiry(time.Now().UTC().Add(-time.Second)) != "expired" {
		t.Fatalf("expected expired")
	}
	if classifyExpiry(time.Now().UTC().Add(5*time.Minute)) != "expiring_soon" {
		t.Fatalf("expected expiring_soon")
	}
	if classifyExpiry(time.Now().UTC().Add(2*time.Hour)) != "valid" {
		t.Fatalf("expected valid")
	}

	if needsRefreshFromStatus("expired") != "yes" || needsRefreshFromStatus("expiring_soon") != "yes" || needsRefreshFromStatus("valid") != "no" || needsRefreshFromStatus("x") != "unknown" {
		t.Fatalf("unexpected needsRefreshFromStatus mapping")
	}

	if statusRank("expired") != 3 || statusRank("expiring_soon") != 2 || statusRank("valid") != 1 || statusRank("x") != 0 {
		t.Fatalf("unexpected statusRank mapping")
	}
}
