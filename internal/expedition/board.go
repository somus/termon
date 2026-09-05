// Package expedition owns Signal Board rotation and support-pool data for solo routes.
package expedition

import "time"

// CycleDays is the server-day rotation length.
const CycleDays = 8

// FamilyOrder is the canonical eight-day Signal Board sequence (24 base Families).
var FamilyOrder = []string{
	"rootkit", "sproutware", "thornpatch", "mossmuff", "rootanami",
	"emberbyte", "cindernode", "scorchip", "wickware",
	"aquabit", "flowcell", "gushkit", "mistcache", "splashscreen",
	"zaplet", "joulpup", "amperent", "surgetail",
	"spamlet", "bloatware", "wormate",
	"chippunk", "coghound", "servoboar",
}

// DayIndex returns the eight-day cycle index for a UTC calendar date.
func DayIndex(day time.Time) int {
	d := day.UTC()
	midnight := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	days := int(midnight.Unix() / (24 * 60 * 60))
	if days < 0 {
		days = -days
	}
	return days % CycleDays
}

// ServerDay truncates t to UTC midnight.
func ServerDay(t time.Time) time.Time {
	d := t.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// FamiliesForDayIndex returns three consecutive Families for cycle index 0–7.
func FamiliesForDayIndex(idx int) []string {
	idx = ((idx % CycleDays) + CycleDays) % CycleDays
	start := idx * 3
	out := make([]string, 3)
	for i := range 3 {
		out[i] = FamilyOrder[start+i]
	}
	return out
}

// FamiliesForDay returns the three Families shown on the Signal Board for a UTC date.
func FamiliesForDay(day time.Time) []string {
	return FamiliesForDayIndex(DayIndex(day))
}

// BoardIndex returns the card index 0–2 for slug on the given UTC day, or -1.
func BoardIndex(day time.Time, slug string) int {
	fams := FamiliesForDay(day)
	for i, f := range fams {
		if f == slug {
			return i
		}
	}
	return -1
}
