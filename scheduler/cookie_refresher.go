package scheduler

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/utils"
	"log"
	"math/rand"
	"strings"
	"time"
)

// StartCookieRefresher 每天下午 18:00~19:00 更新所有学生的 cookies，智能调节间隔
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

			// 总登录时间 1 小时
			totalDuration := time.Hour
			stuCount := len(students)
			var interval time.Duration
			if stuCount > 0 {
				interval = totalDuration / time.Duration(stuCount)
			} else {
				interval = time.Second * 10
			}

			for _, stu := range students {
				cookies, err := student.LoginWithoutBind(stu.StuID, stu.Password)

				// ===== 登录失败或 cookies 为空 =====
				if err != nil || cookies == nil || len(cookies) == 0 {
					log.Printf("⚠️ 登录失败: 学号=%s，错误=%v", stu.StuID, err)

					// 判断是否密码错误且无人绑定 → 删除该学生
					if err != nil && strings.Contains(err.Error(), "用户不存在或密码错误") {
						var count int64
						if e := database.DB.Model(&database.UserStudent{}).
							Where("stu_id = ?", stu.StuID).Count(&count).Error; e != nil {
							log.Printf("❌ 查询绑定关系失败: 学号=%s，错误=%v", stu.StuID, e)
						} else if count == 0 {
							// 无任何绑定 → 删除该学号
							if e := database.DB.Delete(&database.Student{}, "stu_id = ?", stu.StuID).Error; e != nil {
								log.Printf("❌ 删除学生失败: 学号=%s，错误=%v", stu.StuID, e)
							} else {
								log.Printf("🗑️ 学号 %s 因无人绑定且密码错误，已从 student 表移除", stu.StuID)
							}
							time.Sleep(interval + time.Duration(rand.Intn(10))*time.Second)
							continue
						}
					}

					// 有绑定用户 → 必发邮件
					var userStudents []database.UserStudent
					if e := database.DB.Where("stu_id = ?", stu.StuID).Find(&userStudents).Error; e == nil {
						for _, us := range userStudents {
							var user database.User
							if e := database.DB.First(&user, us.UserID).Error; e != nil {
								log.Printf("⚠️ 查询用户失败: UserID=%d, 错误=%v", us.UserID, e)
								continue
							}
							if mailErr := utils.SendAccountErrorEmail(
								user.Email,
								stu.Name,
								stu.StuID,
								err.Error(),
								time.Now(),
							); mailErr != nil {
								log.Printf("❌ 邮件发送失败: 邮箱=%s，错误=%v", user.Email, mailErr)
							} else {
								log.Printf("📧 已通知用户 %s（邮箱=%s）学号 %s 登录失败",
									user.Username, user.Email, stu.StuID)
							}
						}
					} else {
						log.Printf("❌ 查询绑定用户失败: 学号=%s, 错误=%v", stu.StuID, e)
					}

					// 登录失败 → 不更新 cookies，保留旧值
					time.Sleep(interval + time.Duration(rand.Intn(10))*time.Second)
					continue
				}

				// ===== 登录成功才更新 cookies =====
				stu.Cookies = utils.SerializeCookies(cookies)
				stu.LastLogin = time.Now()
				if err := database.DB.Save(&stu).Error; err != nil {
					log.Printf("❌ 保存失败: 学号=%s, 错误=%v", stu.StuID, err)
				} else {
					log.Printf("✅ 学号 %s cookies 已更新", stu.StuID)
				}

				// 间隔休眠，加入随机扰动
				time.Sleep(interval + time.Duration(rand.Intn(10))*time.Second)
			}

			log.Println("✅ 所有学生 cookies 刷新完成")
		}
	}()
}
