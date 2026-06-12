//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	sael "github.com/FasadSalatov/sael/clients/go"
)

type cases struct {
	Version         string `json:"version"`
	QCTTests        []map[string]any `json:"qct_tests"`
	FilterTests     []map[string]any `json:"filter_tests"`
	TracingTests    []map[string]any `json:"tracing_tests"`
	ProtocolTests   []map[string]any `json:"protocol_tests"`
}

func loadCases(t *testing.T) cases {
	t.Helper()
	data, err := os.ReadFile("cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c cases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConformanceQCT(t *testing.T) {
	cases := loadCases(t)
	for _, tc := range cases.QCTTests {
		name := tc["name"].(string)
		t.Run(name, func(t *testing.T) {
			secret := []byte(tc["secret"].(string))
			payloadRaw := tc["payload"].(map[string]any)
			expectValid := tc["expect_valid"].(bool)

			payload := &sael.QCTPayload{
				Issuer:  payloadRaw["iss"].(string),
				Subject: payloadRaw["sub"].(string),
				Expiry:  int64(payloadRaw["exp"].(float64)),
			}
			if nbf, ok := payloadRaw["nbf"].(float64); ok {
				payload.NotBefore = int64(nbf)
			}
			for _, s := range payloadRaw["scope"].([]any) {
				payload.Scope = append(payload.Scope, s.(string))
			}

			token, err := sael.CreateQCT(secret, payload)
			if err != nil && expectValid {
				t.Fatalf("create failed: %v", err)
			}

			_, err = sael.VerifyQCT(token, secret)
			if expectValid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !expectValid {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if expected, ok := tc["expect_error_contains"].(string); ok {
					if !strings.Contains(err.Error(), expected) {
						t.Errorf("expected error to contain %q, got %v", expected, err)
					}
				}
			}
		})
	}
}

func TestConformanceFilter(t *testing.T) {
	cases := loadCases(t)
	for _, tc := range cases.FilterTests {
		name := tc["name"].(string)
		t.Run(name, func(t *testing.T) {
			items := tc["items"].([]any)
			expr := tc["expr"].(string)
			expected := int(tc["expected_count"].(float64))

			result := sael.ApplyFilter(items, expr)
			if len(result) != expected {
				t.Errorf("expected %d items, got %d", expected, len(result))
			}
		})
	}
}

func TestConformanceTracing(t *testing.T) {
	cases := loadCases(t)
	for _, tc := range cases.TracingTests {
		name := tc["name"].(string)
		t.Run(name, func(t *testing.T) {
			expected := int(tc["expected_length"].(float64))
			var got string
			switch name {
			case "trace_id_length":
				got = sael.NewTraceID()
			case "span_id_length":
				got = sael.NewSpanID()
			}
			if len(got) != expected {
				t.Errorf("expected length %d, got %d (%q)", expected, len(got), got)
			}
		})
	}
}
