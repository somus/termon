package dojo

import "time"

// ServerDay truncates t to UTC midnight.
func ServerDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ServerDayString returns yyyy-mm-dd for the UTC calendar date.
func ServerDayString(t time.Time) string {
	return ServerDay(t).Format("2006-01-02")
}

// DailyIndex selects the seven-day Daily Challenge cycle.
func DailyIndex(t time.Time) int {
	day := ServerDay(t)
	return int(day.Unix()/86400) % 7
}
