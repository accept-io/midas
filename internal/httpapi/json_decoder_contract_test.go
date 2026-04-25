package httpapi

// json_decoder_contract_test.go — pins the current strict-runtime-JSON
// decoder contract for POST handlers.
//
// Contracts exercised here:
//
//   readRequestBody (internal/httpapi/server.go):
//     - rejects an empty body with `errors.New("request body must not be empty")`
//     - rejects a whitespace-only body the same way (bytes.TrimSpace before check)
//     - returns errRequestBodyTooLarge when the body exceeds maxBytes
//
//   decodeStrictJSON (internal/httpapi/server.go):
//     - returns `errors.New("invalid JSON payload")` for any decode failure,
//       including:
//         * malformed syntax
//         * wrong JSON type for a struct field
//         * unknown field (DisallowUnknownFields is enabled)
//         * trailing tokens after the first JSON value
//     - returns nil for a single valid JSON value followed only by whitespace
//
//   HTTP handler surface (representative check via /v1/controlplane/promote):
//     - decoder errors surface as HTTP 400 with {"error": "invalid JSON payload"}
//     - empty body surfaces as HTTP 400 with {"error": "request body must not be empty"}
//     - over-large body surfaces as HTTP 413
//
// All tests are passive — they assert the current contract and will fail
// loudly if it silently changes.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/inference"
)

// ---------------------------------------------------------------------------
// decodeStrictJSON — unit-level contract
// ---------------------------------------------------------------------------

type decoderTarget struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestDecodeStrictJSON_ValidSingleObject_NoError(t *testing.T) {
	var v decoderTarget
	if err := decodeStrictJSON([]byte(`{"name":"alice","age":30}`), &v); err != nil {
		t.Fatalf("valid payload must not error; got %v", err)
	}
	if v.Name != "alice" || v.Age != 30 {
		t.Errorf("decode result wrong: %+v", v)
	}
}

func TestDecodeStrictJSON_TrailingWhitespace_NoError(t *testing.T) {
	// A single object followed only by whitespace is valid — the trailing-
	// token check looks for a second JSON value, and whitespace alone makes
	// the second Decode return io.EOF.
	var v decoderTarget
	if err := decodeStrictJSON([]byte(`{"name":"alice","age":30}   `+"\n\n"), &v); err != nil {
		t.Errorf("trailing whitespace must be tolerated; got %v", err)
	}
}

func TestDecodeStrictJSON_MalformedSyntax_Rejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unclosed_object", `{"name":"alice"`},
		{"unclosed_array", `{"tags":["a","b"`},
		{"unquoted_key", `{name:"alice"}`},
		{"unquoted_string_value", `{"name":alice}`},
		{"trailing_comma", `{"name":"alice",}`},
		{"stray_colon", `{"name":"alice":}`},
		{"raw_garbage", `not-json-at-all`},
		{"single_brace", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v decoderTarget
			err := decodeStrictJSON([]byte(tc.body), &v)
			if err == nil {
				t.Fatalf("expected error for malformed JSON %q, got nil", tc.name)
			}
			if err.Error() != "invalid JSON payload" {
				t.Errorf("contract: all decode failures report the generic message; got %q", err.Error())
			}
		})
	}
}

func TestDecodeStrictJSON_WrongType_Rejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"string_in_int_field", `{"name":"alice","age":"thirty"}`},
		{"object_in_int_field", `{"name":"alice","age":{}}`},
		{"bool_in_string_field", `{"name":true,"age":30}`},
		{"array_in_string_field", `{"name":["alice"],"age":30}`},
		{"top_level_array_instead_of_object", `["alice",30]`},
		{"top_level_number_instead_of_object", `42`},
		{"top_level_string_instead_of_object", `"alice"`},
		{"top_level_bool_instead_of_object", `true`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v decoderTarget
			err := decodeStrictJSON([]byte(tc.body), &v)
			if err == nil {
				t.Fatalf("expected error for type-mismatched JSON %q, got nil", tc.name)
			}
			if err.Error() != "invalid JSON payload" {
				t.Errorf("contract: all decode failures report the generic message; got %q", err.Error())
			}
		})
	}
}

func TestDecodeStrictJSON_UnknownField_Rejected(t *testing.T) {
	// DisallowUnknownFields is enabled. Unknown keys at the top level
	// must be rejected. This is the HTTP-API-level contract and is
	// INTENTIONALLY DIFFERENT from the permissive control-plane YAML
	// parser — see internal/controlplane/parser which accepts unknown
	// fields silently.
	cases := []struct {
		name string
		body string
	}{
		{"unknown_top_level_field", `{"name":"alice","age":30,"extra":"drop-me"}`},
		{"only_unknown_field", `{"unknown_key":"value"}`},
		{"typo_of_known_field", `{"nmae":"alice","age":30}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v decoderTarget
			err := decodeStrictJSON([]byte(tc.body), &v)
			if err == nil {
				t.Fatalf("expected error for unknown-field JSON %q, got nil", tc.name)
			}
			if err.Error() != "invalid JSON payload" {
				t.Errorf("contract: all decode failures report the generic message; got %q", err.Error())
			}
		})
	}
}

func TestDecodeStrictJSON_TrailingTokens_Rejected(t *testing.T) {
	// After the first Decode consumes one JSON value, a second Decode must
	// return io.EOF. Any other outcome — another value, parse error on a
	// trailing blob, etc. — is a contract violation.
	cases := []struct {
		name string
		body string
	}{
		{"two_adjacent_objects", `{"name":"alice","age":30}{"name":"bob","age":25}`},
		{"object_then_garbage", `{"name":"alice","age":30}xyz`},
		{"object_then_number", `{"name":"alice","age":30} 99`},
		{"object_then_null", `{"name":"alice","age":30} null`},
		{"object_then_open_brace", `{"name":"alice","age":30} {`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v decoderTarget
			err := decodeStrictJSON([]byte(tc.body), &v)
			if err == nil {
				t.Fatalf("expected error for trailing-token input %q, got nil", tc.name)
			}
			if err.Error() != "invalid JSON payload" {
				t.Errorf("contract: all decode failures report the generic message; got %q", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// readRequestBody — unit-level contract
// ---------------------------------------------------------------------------

func TestReadRequestBody_EmptyBody_Rejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
	rec := httptest.NewRecorder()

	_, err := readRequestBody(rec, req, 1<<20)
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
	if err.Error() != "request body must not be empty" {
		t.Errorf("contract: empty-body message must be stable; got %q", err.Error())
	}
}

func TestReadRequestBody_WhitespaceOnlyBody_RejectedSameAsEmpty(t *testing.T) {
	// readRequestBody calls bytes.TrimSpace before the emptiness check, so
	// a body containing only whitespace is treated identically to an empty
	// body. Pin the message so a future refactor that loses the TrimSpace
	// is caught here.
	cases := []struct {
		name string
		body string
	}{
		{"single_space", " "},
		{"newline_only", "\n"},
		{"tabs_and_newlines", "\t\n\t\n"},
		{"cr_lf", "\r\n\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			_, err := readRequestBody(rec, req, 1<<20)
			if err == nil {
				t.Fatalf("expected error for whitespace-only body %q, got nil", tc.name)
			}
			if err.Error() != "request body must not be empty" {
				t.Errorf("contract: whitespace-only must be treated as empty; got message %q", err.Error())
			}
		})
	}
}

func TestReadRequestBody_OverSizeCap_ReturnsSentinel(t *testing.T) {
	// Body exceeds the maxBytes cap → must return errRequestBodyTooLarge
	// so handlers can map it to HTTP 413.
	const cap = 32
	body := strings.Repeat("x", cap+16)

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	rec := httptest.NewRecorder()

	_, err := readRequestBody(rec, req, int64(cap))
	if err == nil {
		t.Fatal("expected error for oversize body, got nil")
	}
	// Identity match via the exported-for-tests sentinel. Handlers rely
	// on errors.Is(err, errRequestBodyTooLarge) — if this package-level
	// var is ever renamed or replaced by a dynamic error, the handler
	// chain breaks and the wrong HTTP status ships.
	if err != errRequestBodyTooLarge {
		t.Errorf("contract: oversize body must return errRequestBodyTooLarge sentinel; got %v", err)
	}
}

func TestReadRequestBody_Valid_ReturnsBytes(t *testing.T) {
	body := `{"hello":"world"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	rec := httptest.NewRecorder()

	got, err := readRequestBody(rec, req, 1<<20)
	if err != nil {
		t.Fatalf("valid body must not error; got %v", err)
	}
	if string(got) != body {
		t.Errorf("body mismatch: want %q got %q", body, string(got))
	}
}

// ---------------------------------------------------------------------------
// HTTP handler surface — representative end-to-end check.
//
// /v1/controlplane/promote uses both readRequestBody and decodeStrictJSON
// on a strict JSON body. It is a good proxy for the full handler chain
// without dragging in authentication or lifecycle wiring. The assertions
// below encode the HTTP status and body shape that POST handlers using
// these helpers must produce.
// ---------------------------------------------------------------------------

func newDecoderContractServer() *Server {
	// Promote service that returns a successful response regardless of
	// input — this test only cares about the decoder layer, not about
	// promotion semantics. If we ever reach the service, the test has
	// skipped the decoder path entirely and should fail loudly.
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithPromotion(&unexpectedPromotionSvc{})
}

type unexpectedPromotionSvc struct{}

func (u *unexpectedPromotionSvc) Promote(_ context.Context, _ inference.PromoteRequest) (inference.PromoteResponse, error) {
	// Reaching the service means the handler accepted a body it should
	// have rejected at decode time. Return an identifiable error so the
	// assertion message points at the SUT, not at the fake.
	return inference.PromoteResponse{}, fmt.Errorf("UNEXPECTED: decoder allowed bad input through to service")
}

func TestHandlerDecodeFailure_ReturnsBadRequestWithStableErrorMessage(t *testing.T) {
	// Each of these shapes should be caught by decodeStrictJSON and
	// surface as HTTP 400 with the stable error body.
	cases := []struct {
		name string
		body string
	}{
		{"malformed_syntax_unclosed", `{"from":{`},
		{"wrong_type_number_in_object_field", `{"from":42,"to":{"capability_id":"c","process_id":"p"}}`},
		{"unknown_field", `{"from":{"capability_id":"c","process_id":"p"},"to":{"capability_id":"c","process_id":"p"},"unknown":"x"}`},
		{"trailing_object", `{"from":{"capability_id":"c","process_id":"p"},"to":{"capability_id":"c","process_id":"p"}}{"extra":1}`},
		{"trailing_garbage", `{"from":{"capability_id":"c","process_id":"p"},"to":{"capability_id":"c","process_id":"p"}}xyz`},
	}
	srv := newDecoderContractServer()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", []byte(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400 for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
			resp := decodeError(t, rec)
			if resp["error"] != "invalid JSON payload" {
				t.Errorf("contract: decode-fail body must be {\"error\":\"invalid JSON payload\"}; got %v", resp)
			}
		})
	}
}

func TestHandlerEmptyBody_ReturnsBadRequestWithStableErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace_only", "   \n\n   "},
	}
	srv := newDecoderContractServer()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", []byte(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400 for empty/whitespace body, got %d: %s", rec.Code, rec.Body.String())
			}
			resp := decodeError(t, rec)
			if resp["error"] != "request body must not be empty" {
				t.Errorf("contract: empty-body error must be \"request body must not be empty\"; got %v", resp)
			}
		})
	}
}

func TestHandlerOversizeBody_Returns413(t *testing.T) {
	// Write a body larger than maxRequestBodyBytes (1 MiB). The exact
	// status for oversize is 413; the body does not carry a stable
	// message (it reflects the MaxBytesReader error) so we only pin the
	// status code.
	body := strings.Repeat("x", (1<<20)+128)
	srv := newDecoderContractServer()

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", []byte(body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 for oversize body, got %d: %s", rec.Code, rec.Body.String())
	}
}
