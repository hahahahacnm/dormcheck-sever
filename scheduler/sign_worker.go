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

	lastTaskCount := -1 // 初始化为 -1，确保首次能打印

	for range ticker.C {
		tasks, err := GetPendingTasks()
		if err != nil {
			log.Printf("查询任务失败: %v", err)
			continue
		}

		currentCount := len(tasks)

		// 🧠 只有任务数量变化时才打印日志
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
	fiveMinutesAgo := now.Add(-5 * time.Minute) // 避免失败任务立即重试

	var tasks []database.Task

	err := database.DB.
		Where(
			`datetime(date('now') || ' ' || sign_time) <= ? AND 
			 exec_status != ? AND 
			 retry_count < max_retry AND 
			 enabled = ? AND 
			 (executed_at IS NULL OR executed_at <= ?)`,
			now.Format("2006-01-02 15:04:05"), // 当前时间
			"success",
			true,
			fiveMinutesAgo.Format("2006-01-02 15:04:05"), // 至少间隔5分钟
		).
		Find(&tasks).Error

	return tasks, err
}
