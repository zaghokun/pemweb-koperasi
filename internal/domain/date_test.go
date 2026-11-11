package domain

import (
	"errors"
	"testing"
	"time"
)

func TestBusinessDates(t *testing.T) {
	for _, text := range []string{"1000-01-01", "2024-02-29", "9999-12-31"} {
		got, err := ParseBusinessDate(text)
		if err != nil || got.Format("2006-01-02") != text || got.Location() != time.UTC {
			t.Fatalf("ParseBusinessDate(%q) = %v, %v", text, got, err)
		}
	}
	for _, text := range []string{"", "0001-01-01", "0999-12-31", "10000-01-01", "2026-02-29", "2026-13-01", "2026-1-01", "2026-01-1", "2026-01-01T00:00:00Z", " 2026-01-01"} {
		if _, err := ParseBusinessDate(text); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseBusinessDate(%q) error = %v", text, err)
		}
	}
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	input := time.Date(2026, time.January, 31, 0, 15, 0, 0, jakarta)
	got, err := NormalizeBusinessDate(input)
	want := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	if err != nil || !got.Equal(want) {
		t.Fatalf("calendar date shifted: got %v, %v; want %v", got, err, want)
	}
	if _, err := NormalizeBusinessDate(time.Time{}); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("zero date error = %v", err)
	}
}
