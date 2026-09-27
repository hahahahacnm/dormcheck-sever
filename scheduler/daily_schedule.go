package scheduler

import (
	"dormcheck/config"
	"log"
	"time"
)

// startConfiguredDailyJob reads the configured time periodically. If the
// server starts after today's time, it waits until tomorrow instead of
// unexpectedly launching a long-running batch during startup.
func startConfiguredDailyJob(settingKey, fallback string, alreadyRanToday bool, job func()) {
	go func() {
		now := time.Now()
		lastRunDay := ""
		clock := configuredClock(settingKey, fallback)
		scheduledClock, _ := time.Parse("15:04", clock)
		scheduled := time.Date(now.Year(), now.Month(), now.Day(), scheduledClock.Hour(), scheduledClock.Minute(), 0, 0, now.Location())
		skippedAtStartup := !alreadyRanToday && !now.Before(scheduled)
		if alreadyRanToday || skippedAtStartup {
			lastRunDay = now.Format("2006-01-02")
		}

		for {
			now := time.Now()
			today := now.Format("2006-01-02")
			currentClock := configuredClock(settingKey, fallback)
			if skippedAtStartup && currentClock != clock {
				lastRunDay = ""
				skippedAtStartup = false
			}
			clock = currentClock
			scheduledClock, _ := time.Parse("15:04", clock)
			scheduled := time.Date(now.Year(), now.Month(), now.Day(), scheduledClock.Hour(), scheduledClock.Minute(), 0, 0, now.Location())
			if today != lastRunDay && !now.Before(scheduled) {
				lastRunDay = today
				log.Printf("开始执行每日任务 %s（计划时间 %s）", settingKey, clock)
				job()
				continue
			}
			time.Sleep(15 * time.Second)
		}
	}()
}

func configuredClock(settingKey, fallback string) string {
	clock := config.Get(settingKey)
	if _, err := time.Parse("15:04", clock); err != nil {
		return fallback
	}
	return clock
}
