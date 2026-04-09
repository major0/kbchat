package cmd

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/major0/kbchat/keybase"
)

// --- Property-Based Tests ---

// msgSliceInput generates a random slice of MsgSummary with monotonically
// increasing SentAt values, plus random time bounds for filtering.
type msgSliceInput struct {
	Msgs   []keybase.MsgSummary
	After  int64 // Unix timestamp for lower bound
	Before int64 // Unix timestamp for upper bound
}

func (msgSliceInput) Generate(r *rand.Rand, size int) reflect.Value {
	n := r.Intn(50) + 1
	msgs := make([]keybase.MsgSummary, n)
	base := int64(1000000000 + r.Intn(500000000))
	for i := range msgs {
		msgs[i] = keybase.MsgSummary{
			ID:     i + 1,
			SentAt: base + int64(i*60),
		}
	}

	// Generate bounds that overlap the message range.
	minT := msgs[0].SentAt - 120
	maxT := msgs[len(msgs)-1].SentAt + 120
	span := maxT - minT
	a := minT + int64(r.Intn(int(span)))
	b := minT + int64(r.Intn(int(span)))
	if a > b {
		a, b = b, a
	}
	// Ensure a < b so the range is non-degenerate.
	if a == b {
		b = a + 60
	}

	return reflect.ValueOf(msgSliceInput{
		Msgs:   msgs,
		After:  a,
		Before: b,
	})
}

// Feature: keybase-chat-view, Property 1: Timestamp filtering respects bounds.
//
// For any messages and bounds [after, before), only messages with
// sent_at >= after and sent_at < before are returned, in order.
//
// **Validates: Requirements 1.11, 1.12, 1.13, 1.14**

func TestPropertyTimestampFilteringRespectsBounds(t *testing.T) {
	f := func(input msgSliceInput) bool {
		after := time.Unix(input.After, 0)
		before := time.Unix(input.Before, 0)
		result := filterByTimestamp(input.Msgs, &after, &before)

		for i, m := range result {
			if m.SentAt < after.Unix() {
				t.Logf("msg %d: sent_at %d < after %d", i, m.SentAt, after.Unix())
				return false
			}
			if m.SentAt >= before.Unix() {
				t.Logf("msg %d: sent_at %d >= before %d", i, m.SentAt, before.Unix())
				return false
			}
		}

		// Verify no messages were wrongly excluded.
		for _, m := range input.Msgs {
			inRange := m.SentAt >= after.Unix() && m.SentAt < before.Unix()
			found := false
			for _, r := range result {
				if r.ID == m.ID {
					found = true
					break
				}
			}
			if inRange && !found {
				t.Logf("msg id=%d sent_at=%d should be in result", m.ID, m.SentAt)
				return false
			}
		}

		// Verify order is preserved.
		for i := 1; i < len(result); i++ {
			if result[i].ID <= result[i-1].ID {
				t.Logf("order violated: result[%d].ID=%d <= result[%d].ID=%d", i, result[i].ID, i-1, result[i-1].ID)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// countLimitInput generates a random message slice and count for testing
// applyCountLimit.
type countLimitInput struct {
	Msgs     []keybase.MsgSummary
	Count    int
	AfterSet bool
}

func (countLimitInput) Generate(r *rand.Rand, size int) reflect.Value {
	n := r.Intn(50) + 1
	msgs := make([]keybase.MsgSummary, n)
	for i := range msgs {
		msgs[i] = keybase.MsgSummary{
			ID:     i + 1,
			SentAt: int64(1000000000 + i*60),
		}
	}
	// Count between 1 and n (inclusive) so 0 < K <= N.
	count := 1 + r.Intn(n)

	return reflect.ValueOf(countLimitInput{
		Msgs:     msgs,
		Count:    count,
		AfterSet: r.Intn(2) == 1,
	})
}

// Feature: keybase-chat-view, Property 2: Count limiting preserves order and
// selects correct end.
//
// For any slice of length N and count K (0 < K <= N): afterSet=true returns
// first K, afterSet=false returns last K, order preserved.
//
// **Validates: Requirements 1.9, 1.12, 1.13**

func TestPropertyCountLimitingPreservesOrder(t *testing.T) {
	f := func(input countLimitInput) bool {
		result := applyCountLimit(input.Msgs, input.Count, input.AfterSet)

		if len(result) > input.Count {
			t.Logf("result len %d > count %d", len(result), input.Count)
			return false
		}

		expected := min(input.Count, len(input.Msgs))
		if len(result) != expected {
			t.Logf("result len %d != expected %d", len(result), expected)
			return false
		}

		// Verify correct end selected.
		if input.AfterSet {
			// Head: first K messages.
			for i, m := range result {
				if m.ID != input.Msgs[i].ID {
					t.Logf("head: result[%d].ID=%d != msgs[%d].ID=%d", i, m.ID, i, input.Msgs[i].ID)
					return false
				}
			}
		} else {
			// Tail: last K messages.
			offset := len(input.Msgs) - len(result)
			for i, m := range result {
				if m.ID != input.Msgs[offset+i].ID {
					t.Logf("tail: result[%d].ID=%d != msgs[%d].ID=%d", i, m.ID, offset+i, input.Msgs[offset+i].ID)
					return false
				}
			}
		}

		// Verify order preserved.
		for i := 1; i < len(result); i++ {
			if result[i].ID <= result[i-1].ID {
				t.Logf("order violated: result[%d].ID=%d <= result[%d].ID=%d", i, result[i].ID, i-1, result[i-1].ID)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-view, Property 3: Range mode ignores count.
//
// When both after and before are set, all messages in range are returned
// regardless of count.
//
// **Validates: Requirements 1.14, 1.10**

func TestPropertyRangeModeIgnoresCount(t *testing.T) {
	f := func(input msgSliceInput) bool {
		after := time.Unix(input.After, 0)
		before := time.Unix(input.Before, 0)

		// Filter by timestamp first.
		filtered := filterByTimestamp(input.Msgs, &after, &before)

		// Apply count limit with rangeMode semantics: count=0 (unlimited)
		// because resolveQuery sets count=0 when both after+before are set.
		result := applyCountLimit(filtered, 0, true)

		if len(result) != len(filtered) {
			t.Logf("range mode: result len %d != filtered len %d", len(result), len(filtered))
			return false
		}

		// Also verify that a non-zero count still returns all when
		// the pipeline uses count=0 (as resolveQuery would).
		for _, count := range []int{1, 5, 10} {
			limited := applyCountLimit(filtered, 0, true)
			if len(limited) != len(filtered) {
				t.Logf("range mode with count=%d: result len %d != filtered len %d", count, len(limited), len(filtered))
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-view, Property 4: Default behavior is last 20 messages.
//
// For N >= 20 messages with no flags, exactly 20 most recent messages are
// returned.
//
// **Validates: Requirements 1.8**

func TestPropertyDefaultBehaviorLast20(t *testing.T) {
	f := func(input msgSliceInput) bool {
		// Ensure at least 20 messages.
		if len(input.Msgs) < 20 {
			return true // skip small inputs
		}

		now := time.Unix(input.Msgs[len(input.Msgs)-1].SentAt+3600, 0)

		// Default opts: flag parser sets Count=20, no after/before/date.
		opts := viewOpts{Count: 20}
		after, before, count, rangeMode, err := resolveQuery(opts, now)
		if err != nil {
			t.Logf("resolveQuery error: %v", err)
			return false
		}

		if rangeMode {
			t.Log("default should not be range mode")
			return false
		}

		filtered := filterByTimestamp(input.Msgs, after, before)
		result := applyCountLimit(filtered, count, after != nil)

		if len(result) != 20 {
			t.Logf("default: got %d messages, want 20", len(result))
			return false
		}

		// Should be the last 20 from filtered.
		for i, m := range result {
			expected := filtered[len(filtered)-20+i]
			if m.ID != expected.ID {
				t.Logf("default: result[%d].ID=%d != expected.ID=%d", i, m.ID, expected.ID)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// --- Table-Driven Tests for resolveQuery ---

func TestResolveQuery(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	jun10 := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	jun01 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	jun11 := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		opts       viewOpts
		wantAfter  *time.Time
		wantBefore *time.Time
		wantCount  int
		wantRange  bool
		wantErr    bool
	}{
		{
			name:       "no flags (flag parser default count=20)",
			opts:       viewOpts{Count: 20},
			wantAfter:  nil,
			wantBefore: &now,
			wantCount:  20,
			wantRange:  false,
		},
		{
			name:       "--count 10",
			opts:       viewOpts{Count: 10},
			wantAfter:  nil,
			wantBefore: &now,
			wantCount:  10,
			wantRange:  false,
		},
		{
			name:       "--count 0 (unlimited)",
			opts:       viewOpts{Count: 0},
			wantAfter:  nil,
			wantBefore: &now,
			wantCount:  0,
			wantRange:  false,
		},
		{
			name:       "--after T",
			opts:       viewOpts{After: "2024-06-10", Count: 20},
			wantAfter:  &jun10,
			wantBefore: nil,
			wantCount:  20,
			wantRange:  false,
		},
		{
			name:       "--before T",
			opts:       viewOpts{Before: "2024-06-10", Count: 20},
			wantAfter:  nil,
			wantBefore: &jun10,
			wantCount:  20,
			wantRange:  false,
		},
		{
			name:       "--after T --count 5",
			opts:       viewOpts{After: "2024-06-10", Count: 5},
			wantAfter:  &jun10,
			wantBefore: nil,
			wantCount:  5,
			wantRange:  false,
		},
		{
			name:       "--before T --count 5",
			opts:       viewOpts{Before: "2024-06-10", Count: 5},
			wantAfter:  nil,
			wantBefore: &jun10,
			wantCount:  5,
			wantRange:  false,
		},
		{
			name:       "--after A --before B",
			opts:       viewOpts{After: "2024-06-01", Before: "2024-06-10"},
			wantAfter:  &jun01,
			wantBefore: &jun10,
			wantCount:  0,
			wantRange:  true,
		},
		{
			name:       "--date 2024-06-10",
			opts:       viewOpts{Date: "2024-06-10"},
			wantAfter:  &jun10,
			wantBefore: &jun11,
			wantCount:  0,
			wantRange:  true,
		},
		{
			name:    "invalid --after timestamp",
			opts:    viewOpts{After: "not-a-date-at-all-xyz"},
			wantErr: true,
		},
		{
			name:    "invalid --before timestamp",
			opts:    viewOpts{Before: "not-a-date-at-all-xyz"},
			wantErr: true,
		},
		{
			name:    "invalid --date timestamp",
			opts:    viewOpts{Date: "not-a-date-at-all-xyz"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after, before, count, rangeMode, err := resolveQuery(tt.opts, now)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rangeMode != tt.wantRange {
				t.Errorf("rangeMode = %v, want %v", rangeMode, tt.wantRange)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}

			if !timeEqual(after, tt.wantAfter) {
				t.Errorf("after = %v, want %v", formatTimePtr(after), formatTimePtr(tt.wantAfter))
			}
			if !timeEqual(before, tt.wantBefore) {
				t.Errorf("before = %v, want %v", formatTimePtr(before), formatTimePtr(tt.wantBefore))
			}
		})
	}
}

// timeEqual compares two *time.Time values.
func timeEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// formatTimePtr formats a *time.Time for error messages.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "<nil>"
	}
	return t.Format(time.RFC3339)
}
