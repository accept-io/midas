package audit

// cursor_test.go — D30j cursor codec pins.

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestListCursor_EncodeDecodeRoundTrip_Desc(t *testing.T) {
	want := &ListCursor{
		OccurredAt: time.Date(2026, 1, 1, 12, 34, 56, 789, time.UTC),
		SequenceNo: 7,
		ID:         "evt-abc123",
	}
	encoded := EncodeListCursor(want, true)
	if encoded == "" {
		t.Fatal("EncodeListCursor must produce non-empty for non-nil cursor")
	}
	got, err := DecodeListCursor(encoded, true)
	if err != nil {
		t.Fatalf("DecodeListCursor: %v", err)
	}
	if !got.OccurredAt.Equal(want.OccurredAt) {
		t.Errorf("OccurredAt: want %v, got %v", want.OccurredAt, got.OccurredAt)
	}
	if got.SequenceNo != want.SequenceNo {
		t.Errorf("SequenceNo: want %d, got %d", want.SequenceNo, got.SequenceNo)
	}
	if got.ID != want.ID {
		t.Errorf("ID: want %q, got %q", want.ID, got.ID)
	}
}

func TestListCursor_EncodeDecodeRoundTrip_Asc(t *testing.T) {
	want := &ListCursor{
		OccurredAt: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		SequenceNo: 1,
		ID:         "evt-d30j",
	}
	encoded := EncodeListCursor(want, false)
	got, err := DecodeListCursor(encoded, false)
	if err != nil {
		t.Fatalf("DecodeListCursor: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID: want %q, got %q", want.ID, got.ID)
	}
}

func TestListCursor_Encode_NilReturnsEmpty(t *testing.T) {
	if got := EncodeListCursor(nil, true); got != "" {
		t.Errorf("EncodeListCursor(nil) must return empty; got %q", got)
	}
}

func TestListCursor_Decode_EmptyReturnsError(t *testing.T) {
	if _, err := DecodeListCursor("", true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("empty cursor must produce ErrInvalidCursor; got %v", err)
	}
	if _, err := DecodeListCursor("   ", true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("whitespace-only cursor must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_MalformedBase64(t *testing.T) {
	if _, err := DecodeListCursor("!!!not-base64!!!", true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("malformed base64 must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_MalformedJSON(t *testing.T) {
	// Valid base64, invalid JSON body.
	garbage := base64.RawURLEncoding.EncodeToString([]byte("{not json"))
	if _, err := DecodeListCursor(garbage, true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("malformed JSON must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_UnsupportedVersion(t *testing.T) {
	payload := `{"v":999,"occurred_at":"2026-01-01T00:00:00Z","sequence_no":1,"id":"x","order":"desc"}`
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, err := DecodeListCursor(enc, true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("unsupported version must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_MissingID(t *testing.T) {
	payload := `{"v":1,"occurred_at":"2026-01-01T00:00:00Z","sequence_no":1,"id":"","order":"desc"}`
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, err := DecodeListCursor(enc, true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("missing id must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_MissingOccurredAt(t *testing.T) {
	payload := `{"v":1,"occurred_at":"0001-01-01T00:00:00Z","sequence_no":1,"id":"x","order":"desc"}`
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, err := DecodeListCursor(enc, true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("zero occurred_at must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_InvalidOrderValue(t *testing.T) {
	payload := `{"v":1,"occurred_at":"2026-01-01T00:00:00Z","sequence_no":1,"id":"x","order":"sideways"}`
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, err := DecodeListCursor(enc, true); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("invalid order value must produce ErrInvalidCursor; got %v", err)
	}
}

func TestListCursor_Decode_OrderMismatch(t *testing.T) {
	// desc-encoded cursor replayed against asc query.
	encoded := EncodeListCursor(&ListCursor{
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SequenceNo: 1,
		ID:         "evt-x",
	}, true)
	if _, err := DecodeListCursor(encoded, false); !errors.Is(err, ErrCursorOrderMismatch) {
		t.Errorf("order mismatch must produce ErrCursorOrderMismatch; got %v", err)
	}
}

// TestListCursor_EncodedIsOpaque pins that the encoded cursor is
// URL-safe (no padding, no unsafe chars) so it round-trips through
// query strings without escaping.
func TestListCursor_EncodedIsOpaque(t *testing.T) {
	enc := EncodeListCursor(&ListCursor{
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SequenceNo: 1,
		ID:         "evt-y",
	}, true)
	for _, ch := range enc {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			t.Errorf("encoded cursor must be URL-safe; found %q in %q", ch, enc)
		}
	}
	if strings.Contains(enc, "=") {
		t.Errorf("encoded cursor must not include base64 padding; got %q", enc)
	}
}
