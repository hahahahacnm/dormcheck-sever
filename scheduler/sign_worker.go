// scheduler/sign_worker.go
package scheduler

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"log"
	"time"
)

// StartWorker 启动签到调度器（每分钟执行一次）
func StartWorker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	lastTaskCount := -1

	for range ticker.C {
		tasks, err := GetPendingTasks()
		if err != nil {
			log.Printf("查询任务失败: %v", err)
			continue
		}

		currentCount := len(tasks)

		if currentCount != lastTaskCount {
			log.Println("⏰ 自动签到调度器运行中...")
			if currentCount == 0 {
				log.Println("📭 当前无待签到任务（检测持续运行中... ...）")
			} else {
				log.Printf("📌 找到 %d 个待签到任务，准备开始执行！", currentCount)
			}
			lastTaskCount = currentCount
		}

		for _, task := range tasks {
			log.Printf("→ 执行签到任务: StuID=%s, ActivityID=%s", task.StuID, task.ActivityID)
			err := student.ExecuteSignTask(&task)
			if err != nil {
				log.Printf("❌ 执行失败: %v", err)
			} else {
				log.Printf("✅ 执行完成（结果已由任务内部判定）: ActivityID=%s", task.ActivityID)
			}
		}
	}
}

// GetPendingTasks 获取所有待签到任务
func GetPendingTasks() ([]database.Task, error) {
	now := time.Now()
	currentTime := now.Format("15:04:05")
	fiveMinutesAgo := now.Add(-5 * time.Minute).Format("2006-01-02 15:04:05")

	var tasks []database.Task

	err := database.DB.
		Where(`
			enabled = ? AND
			exec_status != ? AND
			retry_count < max_retry AND
			sign_time <= ? AND
			(executed_at IS NULL OR executed_at <= ?)
		`, true, "success", currentTime, fiveMinutesAgo).
		Find(&tasks).Error

	return tasks, err
}

