package domain

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}

	return t
}

func days(ss ...string) []time.Time {
	out := make([]time.Time, 0, len(ss))
	for _, s := range ss {
		out = append(out, day(s))
	}

	return out
}

func TestLocalDay(t *testing.T) {
	// 20:00 UTC on Oct 3 is already Oct 4 in Tashkent.
	if got := LocalDay(time.Date(2026, 10, 3, 20, 0, 0, 0, time.UTC)); !got.Equal(day("2026-10-04")) {
		t.Fatalf("LocalDay = %v", got)
	}

	if got := LocalDay(time.Date(2026, 10, 3, 18, 59, 0, 0, time.UTC)); !got.Equal(day("2026-10-03")) {
		t.Fatalf("LocalDay = %v", got)
	}
}

func TestComputeStreak(t *testing.T) {
	today := day("2026-10-08") // a Thursday; the week started Monday Oct 5

	cases := []struct {
		name string
		days []time.Time
		want Streak
	}{
		{"none", nil, Streak{}},
		{"today only", days("2026-10-08"), Streak{Current: 1, Longest: 1, CompletedToday: true, ThisWeek: 1}},
		{"run up to today", days("2026-10-08", "2026-10-07", "2026-10-06", "2026-10-04"),
			Streak{Current: 3, Longest: 3, CompletedToday: true, ThisWeek: 3}},
		{"run up to yesterday is kept", days("2026-10-07", "2026-10-06"),
			Streak{Current: 2, Longest: 2, ThisWeek: 2}},
		{"missed a whole day", days("2026-10-06", "2026-10-05", "2026-10-04", "2026-10-03"),
			Streak{Current: 0, Longest: 4, ThisWeek: 2}},
		{"longest is older", days("2026-10-08", "2026-10-01", "2026-09-30", "2026-09-29"),
			Streak{Current: 1, Longest: 3, CompletedToday: true, ThisWeek: 1}},
	}

	for _, tc := range cases {
		got := ComputeStreak(tc.days, today, 7)
		tc.want.Total = 7

		if len(tc.days) > 0 {
			tc.want.LastCompletedOn = &tc.days[0]
		}

		if got.Current != tc.want.Current || got.Longest != tc.want.Longest || got.CompletedToday != tc.want.CompletedToday ||
			got.ThisWeek != tc.want.ThisWeek || got.Total != tc.want.Total ||
			(got.LastCompletedOn == nil) != (tc.want.LastCompletedOn == nil) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestComputeStreak_SundayWeek(t *testing.T) {
	// On a Sunday the week still starts on the Monday before.
	got := ComputeStreak(days("2026-10-11", "2026-10-05", "2026-10-04"), day("2026-10-11"), 3)
	if got.ThisWeek != 2 {
		t.Fatalf("ThisWeek = %d, want 2", got.ThisWeek)
	}
}
