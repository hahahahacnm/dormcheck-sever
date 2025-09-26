package scheduler

import (
	"dormcheck/database"
	"log"
	"time"
)

// StartResetWorker 启动每日 00:05 重置任务状态的定时器
func StartResetWorker() {
	go func() {
		for {
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, now.Location())
			duration := nextMidnight.Sub(now)

			// 睡到凌晨 00:03
			time.Sleep(duration)

			// ✅ 重置所有任务
			err := database.DB.
				Model(&database.Task{}).
				Where("1 = 1").
				Updates(map[string]interface{}{
					"retry_count": 0,
					"exec_status": "pending",
					"last_error":  "",
					"executed_at": nil, // ✅ 补充清空执行时间
				}).Error

			if err != nil {
				log.Printf("❌ 每日任务重置失败: %v", err)
			} else {
				log.Println("✅ 所有任务已于 00:05 重置")
			}
		}
	}()
}
