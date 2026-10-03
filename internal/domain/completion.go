package domain

import "time"

// MaxCompletionNoteLength is the longest reflection note, in characters (activity_completions_note_len_chk).
const MaxCompletionNoteLength = 1000

// Location is the time zone that decides which calendar day a completion belongs to. Uzbekistan
// is UTC+5 all year (no daylight saving), so a fixed zone needs no tzdata.
var Location = time.FixedZone("UZT", int((5 * time.Hour).Seconds())) //nolint:mnd // UTC+5

// Completion records that a child did an activity on a day, with the caregiver's optional reflection.
type Completion struct {
	ID          string
	ChildID     string
	ActivityID  string
	UserID      *string
	CompletedOn time.Time // a date: midnight UTC of the local day
	Note        *string
	CreatedAt   time.Time
}

// CompletionFilter pages through one child's completions.
type CompletionFilter struct {
	ChildID string
	Limit   int
	Offset  int
}

// Streak summarises a child's practice.
type Streak struct {
	Current         int        // consecutive days up to today, or up to yesterday if today has no completion yet
	Longest         int        // longest run of consecutive days ever
	CompletedToday  bool       // at least one completion today
	ThisWeek        int        // distinct days with a completion this week (Monday to today)
	Total           int        // all completions
	LastCompletedOn *time.Time // nil before the first completion
}

// LocalDay returns the calendar day of t in Location, as midnight UTC of that date.
func LocalDay(t time.Time) time.Time {
	y, m, d := t.In(Location).Date()

	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ComputeStreak builds a Streak from the distinct days that have a completion, newest first, as
// returned by LocalDay. today is LocalDay(now); total is the number of completions.
func ComputeStreak(days []time.Time, today time.Time, total int) Streak {
	s := Streak{Total: total}
	if len(days) == 0 {
		return s
	}

	last := days[0]
	s.LastCompletedOn = &last
	s.CompletedToday = last.Equal(today)

	weekStart := today.AddDate(0, 0, -daysSinceMonday(today))

	run := 0

	for i, d := range days {
		if !d.Before(weekStart) && !d.After(today) {
			s.ThisWeek++
		}

		if i > 0 && days[i-1].AddDate(0, 0, -1).Equal(d) {
			run++
		} else {
			run = 1
		}

		s.Longest = max(s.Longest, run)

		// The current streak is the first run, as long as it reaches today or yesterday.
		if run == i+1 && !last.Before(today.AddDate(0, 0, -1)) {
			s.Current = run
		}
	}

	return s
}

// daysSinceMonday is 0 on Monday and 6 on Sunday.
func daysSinceMonday(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 6 //nolint:mnd // Sunday ends the week
	}

	return int(t.Weekday() - time.Monday)
}
