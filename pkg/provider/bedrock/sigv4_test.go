package bedrock

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func newSignedRequest(t *testing.T, signer *SigV4Signer, when time.Time) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-opus-4-8/invoke",
		strings.NewReader(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := signer.Sign(req, []byte(`{"hello":"world"}`), "bedrock", "us-east-1", when); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return req
}

func TestSigV4SignSetsHeaders(t *testing.T) {
	signer, err := NewSigV4Signer(Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET"})
	if err != nil {
		t.Fatalf("NewSigV4Signer: %v", err)
	}
	when := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	req := newSignedRequest(t, signer, when)

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Errorf("Authorization prefix wrong: %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKID/20260710/us-east-1/bedrock/aws4_request") {
		t.Errorf("scope wrong in Authorization: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date") {
		t.Errorf("signed headers wrong: %q", auth)
	}
	if req.Header.Get("X-Amz-Date") != "20260710T120000Z" {
		t.Errorf("X-Amz-Date = %q", req.Header.Get("X-Amz-Date"))
	}
	if req.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Error("X-Amz-Content-Sha256 not set")
	}
}

func TestSigV4Deterministic(t *testing.T) {
	signer, _ := NewSigV4Signer(Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET"})
	when := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	a := newSignedRequest(t, signer, when).Header.Get("Authorization")
	b := newSignedRequest(t, signer, when).Header.Get("Authorization")
	if a != b {
		t.Errorf("signing not deterministic:\n%s\n%s", a, b)
	}
}

func TestSigV4SessionToken(t *testing.T) {
	signer, _ := NewSigV4Signer(Credentials{
		AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "TOKEN",
	})
	req := newSignedRequest(t, signer, time.Now())
	if req.Header.Get("X-Amz-Security-Token") != "TOKEN" {
		t.Error("session token header not set")
	}
}

func TestSigV4RequiresCredentials(t *testing.T) {
	if _, err := NewSigV4Signer(Credentials{}); err == nil {
		t.Error("expected error for empty credentials")
	}
}
