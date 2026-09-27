package scheduler

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"log"
	"time"
)

// Reset previous-day results on startup and each minute after midnight.
// A task in a window crossing midnight keeps its result until the window ends.
func StartResetWorker() {
	go func() {
		clearStaleTaskClaims(time.Now())
		deleteCanceledTasksAfterClaims()
		resetPreviousDayTasks(time.Now())
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			clearStaleTaskClaims(time.Now())
			deleteCanceledTasksAfterClaims()
			resetPreviousDayTasks(time.Now())
		}
	}()
}

func deleteCanceledTasksAfterClaims() {
	result := database.DB.Where("cancel_after_run = TRUE AND running_at IS NULL").Delete(&database.Task{})
	if result.Error != nil {
		log.Printf("清理已封禁取消的任务失败: %v", result.Error)
	}
}

func clearStaleTaskClaims(now time.Time) {
	if err := database.DB.Model(&database.Task{}).Where("running_at < ?", now.Add(-student.TaskClaimStaleAfter)).Update("running_at", nil).Error; err != nil {
		log.Printf("清理过期任务执行标记失败: %v", err)
	}
}

func resetPreviousDayTasks(now time.Time) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var stale []database.Task
	if err := database.DB.Where("running_at IS NULL AND (retry_count <> 0 OR exec_status <> ?) AND (executed_at IS NULL OR executed_at < ?)", "pending", dayStart).Find(&stale).Error; err != nil {
		log.Printf("查询上日任务状态失败: %v", err)
		return
	}
	for _, task := range stale {
		_, _, windowDay, active := student.TaskWindowBounds(task, now)
		if active && windowDay.Before(dayStart) {
			continue
		}
		result := database.DB.Model(&database.Task{}).
			Where("id = ? AND running_at IS NULL AND (executed_at IS NULL OR executed_at < ?)", task.ID, dayStart).
			Updates(map[string]interface{}{"retry_count": 0, "exec_status": "pending", "last_error": "", "executed_at": nil})
		if result.Error != nil {
			log.Printf("重置过期任务状态失败: task=%d: %v", task.ID, result.Error)
		}
	}
}
