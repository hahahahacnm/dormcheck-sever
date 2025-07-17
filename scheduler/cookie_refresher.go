package scheduler

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/utils"
	"log"
	"time"
)

// StartCookieRefresher 每天下午 18:00 更新所有学生的 cookies
func StartCookieRefresher() {
	go func() {
		for {
			now := time.Now()
			nextRefresh := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location())
			if now.After(nextRefresh) {
				nextRefresh = nextRefresh.Add(24 * time.Hour)
			}

			time.Sleep(nextRefresh.Sub(now))

			log.Println("🔄 正在刷新所有学生 cookies...")

			var students []database.Student
			if err := database.DB.Find(&students).Error; err != nil {
				log.Printf("❌ 查询学生失败: %v", err)
				continue
			}

			for _, stu := range students {
				cookies, err := student.LoginWithoutBind(stu.StuID, stu.Password)
				if err != nil {
					log.Printf("⚠️ 登录失败: 学号=%s，错误=%v", stu.StuID, err)
					continue
				}

				stu.Cookies = utils.SerializeCookies(cookies)
				stu.LastLogin = time.Now()

				if err := database.DB.Save(&stu).Error; err != nil {
					log.Printf("❌ 保存失败: 学号=%s, 错误=%v", stu.StuID, err)
				} else {
					log.Printf("✅ 学号 %s cookies 已更新", stu.StuID)
				}
			}

			log.Println("✅ 所有学生 cookies 刷新完成")
		}
	}()
}
