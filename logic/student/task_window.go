package student

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"dormcheck/database"
)

var ErrManualRunOutsideWindow = errors.New("当前不在活动允许的执行时间内")

var activityClockPattern = regexp.MustCompile(`\d{1,2}:\d{2}`)

// AutomaticSignTime schedules roughly five minutes after opening. Short
// windows are scheduled earlier so the planned attempt remains inside them.
func AutomaticSignTime(start, end string) string {
	parse := func(value string) (time.Time, bool) {
		for _, layout := range []string{"15:04:05", "15:04"} {
			if clock, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
				return clock, true
			}
		}
		return time.Time{}, false
	}
	begin, okBegin := parse(start)
	finish, okEnd := parse(end)
	if !okBegin || !okEnd {
		return ""
	}
	if !finish.After(begin) {
		finish = finish.Add(24 * time.Hour)
	}
	window := finish.Sub(begin)
	delay := 5 * time.Minute
	if window <= 10*time.Minute {
		delay = window / 2
	}
	return begin.Add(delay).Format("15:04")
}

// IsTaskWithinExecutionWindow reports whether now is inside the task's scheduled
// execution window and activity date range. It also supports windows crossing midnight.
func IsTaskWithinExecutionWindow(task database.Task, now time.Time) bool {
	_, _, _, ok := TaskWindowBounds(task, now)
	return ok
}

// TaskWindowBounds returns the active upstream window and the date on which
// that window began. The latter is stable for windows crossing midnight.
func TaskWindowBounds(task database.Task, now time.Time) (time.Time, time.Time, time.Time, bool) {
	clocks := activityClockPattern.FindAllString(task.ActivityTimeRange, -1)
	if len(clocks) < 2 {
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	// Manual runs are allowed throughout the upstream activity window, including
	// the first five minutes before the automatic schedule begins.
	startClock, err := time.ParseInLocation("15:04", clocks[0], now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	endClock, err := time.ParseInLocation("15:04", clocks[1], now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, false
	}

	for _, dayOffset := range []int{0, -1} {
		day := time.Date(now.Year(), now.Month(), now.Day()+dayOffset, 0, 0, 0, 0, now.Location())
		start := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, now.Location())
		end := time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), 0, 0, now.Location())
		if !end.After(start) {
			end = end.Add(24 * time.Hour)
		}
		if now.Before(start) || now.After(end) || !taskActivityDateAllowed(task, start) {
			continue
		}
		return start, end, day, true
	}
	return time.Time{}, time.Time{}, time.Time{}, false
}

func taskActivityDateAllowed(task database.Task, scheduledDay time.Time) bool {
	day := scheduledDay.Format("2006-01-02")
	if strings.TrimSpace(task.ActivityStartDate) != "" {
		start, ok := activityDate(task.ActivityStartDate)
		if !ok || day < start {
			return false
		}
	}
	if strings.TrimSpace(task.ActivityEndDate) != "" {
		end, ok := activityDate(task.ActivityEndDate)
		if !ok || day > end {
			return false
		}
	}
	return true
}

func activityDate(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		if parsed, err := time.Parse("2006-01-02", strings.ReplaceAll(value[:10], "/", "-")); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	return "", false
}
