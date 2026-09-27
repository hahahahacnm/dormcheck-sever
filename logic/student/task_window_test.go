package student

import (
	"testing"
	"time"

	"dormcheck/database"
)

func TestTaskWindowBounds(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	cases := []struct {
		name      string
		task      database.Task
		now       time.Time
		want      bool
		windowDay string
	}{
		{"manual before automatic start", database.Task{ActivityTimeRange: "21:30:00-22:30:00", SignTime: "21:35", ActivityStartDate: "2026-09-25", ActivityEndDate: "2026-09-26"}, time.Date(2026, 9, 26, 21, 32, 0, 0, location), true, "2026-09-26"},
		{"expired date", database.Task{ActivityTimeRange: "21:30-22:30", SignTime: "21:35", ActivityEndDate: "2026/09/25"}, time.Date(2026, 9, 26, 21, 40, 0, 0, location), false, ""},
		{"cross midnight", database.Task{ActivityTimeRange: "23:30-00:30", SignTime: "23:35", ActivityStartDate: "2026-09-25", ActivityEndDate: "2026-09-25"}, time.Date(2026, 9, 26, 0, 10, 0, 0, location), true, "2026-09-25"},
		{"malformed date fails closed", database.Task{ActivityTimeRange: "21:30-22:30", ActivityEndDate: "wrong"}, time.Date(2026, 9, 26, 21, 40, 0, 0, location), false, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, day, ok := TaskWindowBounds(test.task, test.now)
			if ok != test.want {
				t.Fatalf("window open = %v, want %v", ok, test.want)
			}
			if ok && day.Format("2006-01-02") != test.windowDay {
				t.Fatalf("window day = %s, want %s", day.Format("2006-01-02"), test.windowDay)
			}
		})
	}
}

func TestAutomaticSignTimeFitsShortWindow(t *testing.T) {
	if got := AutomaticSignTime("21:30:00", "21:35:00"); got != "21:32" {
		t.Fatalf("short window scheduled at %s", got)
	}
	if got := AutomaticSignTime("21:30:00", "22:30:00"); got != "21:35" {
		t.Fatalf("normal window scheduled at %s", got)
	}
}
