package bedrock

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Credentials are AWS access credentials used to sign requests. SessionToken is
// optional and set only for temporary (STS) credentials.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// SigV4Signer signs requests using the AWS Signature Version 4 algorithm.
// It implements Signer. It is safe for concurrent use.
type SigV4Signer struct {
	creds Credentials
}

// NewSigV4Signer builds a signer from static credentials.
func NewSigV4Signer(creds Credentials) (*SigV4Signer, error) {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("bedrock: access key id and secret access key are required")
	}
	return &SigV4Signer{creds: creds}, nil
}

const (
	algorithm    = "AWS4-HMAC-SHA256"
	timeFormat   = "20060102T150405Z"
	dateFormat   = "20060102"
	terminator   = "aws4_request"
	emptyPayload = "" // sentinel for readability
)

// Sign adds the x-amz-date, (optional) x-amz-security-token, and Authorization
// headers to req per the AWS SigV4 specification.
func (s *SigV4Signer) Sign(req *http.Request, payload []byte, service, region string, t time.Time) error {
	t = t.UTC()
	amzDate := t.Format(timeFormat)
	dateStamp := t.Format(dateFormat)

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	if s.creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.creds.SessionToken)
	}

	payloadHash := hexSHA256(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders, canonicalHeaders := canonicalizeHeaders(req)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL.EscapedPath()),
		canonicalQuery(req.URL.RawQuery),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, region, service, terminator}, "/")
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := s.deriveSigningKey(dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authorization := fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, s.creds.AccessKeyID, scope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authorization)
	return nil
}

func (s *SigV4Signer) deriveSigningKey(dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.creds.SecretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte(terminator))
}

// canonicalizeHeaders returns the semicolon-joined signed-header list and the
// canonical header block. Host, Content-Type, and all x-amz-* headers are
// signed.
func canonicalizeHeaders(req *http.Request) (signedHeaders, canonicalHeaders string) {
	names := make([]string, 0, len(req.Header)+1)
	values := make(map[string]string)

	add := func(name, value string) {
		lower := strings.ToLower(name)
		names = append(names, lower)
		values[lower] = strings.TrimSpace(value)
	}

	add("host", req.URL.Host)
	for name, vs := range req.Header {
		lower := strings.ToLower(name)
		if lower == "host" {
			continue
		}
		if lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			add(name, strings.Join(vs, ","))
		}
	}

	sort.Strings(names)
	var block strings.Builder
	for _, n := range names {
		block.WriteString(n)
		block.WriteString(":")
		block.WriteString(values[n])
		block.WriteString("\n")
	}
	return strings.Join(names, ";"), block.String()
}

// canonicalURI returns the URI-encoded path. The path is already escaped by
// url.URL; an empty path canonicalizes to "/".
func canonicalURI(escapedPath string) string {
	if escapedPath == "" {
		return "/"
	}
	return escapedPath
}

// canonicalQuery sorts and re-encodes the query string per SigV4 rules.
func canonicalQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	pairs := strings.Split(rawQuery, "&")
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
