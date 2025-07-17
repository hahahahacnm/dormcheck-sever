package scheduler

import (
	"dormcheck/database"
	"log"
	"time"
)

// StartResetWorker 启动每日 00:00 重置任务状态的定时器
func StartResetWorker() {
	go func() {
		for {
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 3, 0, 0, now.Location())
			duration := nextMidnight.Sub(now)

			// 睡到凌晨
			time.Sleep(duration)

			// ✅ 重置所有任务（必须加 Where("1=1")）
			err := database.DB.
				Model(&database.Task{}).
				Where("1 = 1").
				Updates(map[string]interface{}{
					"retry_count": 0,
					"exec_status": "pending",
					"last_error":  "",
				}).Error

			if err != nil {
				log.Printf("❌ 每日任务重置失败: %v", err)
			} else {
				log.Println("✅ 所有任务已于 03:00 重置")
			}
		}
	}()
}
