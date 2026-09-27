// scheduler/sign_worker.go
package scheduler

import (
	"crypto/sha1"
	"dormcheck/config"
	"dormcheck/database"
	"dormcheck/logic/student"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
)

// StartWorker 启动签到调度器（每分钟执行一次）
func StartWorker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	lastTaskCount := -1

	firstRun := true
	for {
		if !firstRun {
			<-ticker.C
		}
		firstRun = false
		tasks, err := GetPendingTasks()
		if err != nil {
			log.Printf("查询任务失败: %v", err)
			continue
		}
		now := time.Now()
		due := tasks[:0]
		for _, task := range tasks {
			if isTaskDue(task, now) {
				due = append(due, task)
			}
		}
		tasks = due

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

		limit := config.GetInt("task_batch_size", 10)
		if limit < 1 {
			limit = 1
		}
		if len(tasks) > limit {
			tasks = tasks[:limit]
		}
		var wg sync.WaitGroup
		for _, item := range tasks {
			task := item
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Printf("→ 执行签到任务: StuID=%s, ActivityID=%s", task.StuID, task.ActivityID)
				if err := student.ExecuteSignTask(&task); err != nil {
					log.Printf("❌ 执行失败: %v", err)
				}
			}()
		}
		wg.Wait()
	}
}

// GetPendingTasks 获取所有待签到任务
func GetPendingTasks() ([]database.Task, error) {
	now := time.Now()
	lastAttemptCutoff := now.Add(-time.Minute)

	var tasks []database.Task

	err := database.DB.
		Table("tasks AS t").Select("t.*").
		Where(`
			t.enabled = ? AND
			t.running_at IS NULL AND
			(t.activity_state IN ('', 'normal') OR t.activity_override = TRUE) AND
			NOT EXISTS (SELECT 1 FROM student_bans sb WHERE sb.stu_id = t.stu_id AND (sb.expires_at IS NULL OR sb.expires_at > ?)) AND
			EXISTS (SELECT 1 FROM user_students owner JOIN users owner_user ON owner_user.id = owner.user_id
				WHERE owner.stu_id = t.stu_id AND
				(owner_user.banned = FALSE OR (owner_user.ban_expires_at IS NOT NULL AND owner_user.ban_expires_at <= ?))) AND
			t.exec_status != ? AND
			t.retry_count < ? AND
			(t.executed_at IS NULL OR t.executed_at <= ?)
		`, true, now, now, "success", config.GetInt("task_max_retries", 3)+1, lastAttemptCutoff).
		Order(`CASE WHEN EXISTS (SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id WHERE us.stu_id = t.stu_id AND u.role IN (0, 2, 3) AND (u.banned = FALSE OR (u.ban_expires_at IS NOT NULL AND u.ban_expires_at <= NOW()))) THEN 0 ELSE 1 END, t.sign_time ASC, t.id ASC`).
		Find(&tasks).Error

	return tasks, err
}

func isTaskDue(task database.Task, now time.Time) bool {
	windowStart, _, windowDay, ok := student.TaskWindowBounds(task, now)
	if !ok {
		return false
	}
	base, err := time.ParseInLocation("15:04", task.SignTime, now.Location())
	if err != nil {
		return false
	}
	scheduled := time.Date(windowDay.Year(), windowDay.Month(), windowDay.Day(), base.Hour(), base.Minute(), 0, 0, now.Location())
	if scheduled.Before(windowStart) {
		scheduled = scheduled.Add(24 * time.Hour)
	}
	seed := sha1.Sum([]byte(fmt.Sprintf("%d:%s", task.ID, windowDay.Format("2006-01-02"))))
	spread := int(binary.BigEndian.Uint16(seed[:2]))
	jitter := time.Duration(spread%61-30) * time.Second
	return !now.Before(scheduled.Add(jitter))
}
