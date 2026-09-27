package scheduler

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/utils"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var activityAuditMu sync.Mutex

type activityAuditNotice struct {
	Email        string
	StudentName  string
	StuID        string
	ActivityID   string
	ActivityName string
	Issue        string
	Recovered    bool
}

// StartActivityAuditor checks tasks once a day, away from the evening sign-in peak.
func StartActivityAuditor() <-chan struct{} {
	ready := make(chan struct{})
	go auditActivities(ready)
	startConfiguredDailyJob("activity_audit_time", "04:00", false, func() { auditActivities(nil) })
	return ready
}

func auditActivities(ready chan<- struct{}) {
	activityAuditMu.Lock()
	defer activityAuditMu.Unlock()
	defer func() {
		if ready != nil {
			close(ready)
		}
	}()
	var tasks []database.Task
	if err := database.DB.Order("stu_id ASC, activity_id ASC").Find(&tasks).Error; err != nil {
		log.Printf("查询待审计签到任务失败: %v", err)
		return
	}
	tasksByActivity := make(map[string][]database.Task)
	studentsByActivity := make(map[string]map[string]struct{})
	activityIDsByStudent := make(map[string]map[string]struct{})
	for _, task := range tasks {
		id, err := strconv.Atoi(task.ActivityID)
		if err != nil {
			log.Printf("活动 ID 无法审计，保留原任务: task=%d activity=%q", task.ID, task.ActivityID)
			continue
		}
		activityID := strconv.Itoa(id)
		tasksByActivity[activityID] = append(tasksByActivity[activityID], task)
		if studentsByActivity[activityID] == nil {
			studentsByActivity[activityID] = make(map[string]struct{})
		}
		studentsByActivity[activityID][task.StuID] = struct{}{}
		if activityIDsByStudent[task.StuID] == nil {
			activityIDsByStudent[task.StuID] = make(map[string]struct{})
		}
		activityIDsByStudent[task.StuID][activityID] = struct{}{}
	}

	// A single account often sees the same faculty-wide activities as dozens of
	// other students. Query the account covering the most unresolved activities
	// first, then query other accounts only for IDs still not found.
	resolved := make(map[string]schoolActivity)
	queried := make(map[string]bool)
	studentHasConclusiveList := make(map[string]bool)
	conclusive := make(map[string]bool)
	querySuccesses := 0
	studentIDs := make([]string, 0, len(activityIDsByStudent))
	for stuID := range activityIDsByStudent {
		studentIDs = append(studentIDs, stuID)
	}
	sort.Strings(studentIDs)
	for len(resolved) < len(tasksByActivity) {
		selectedStudent := ""
		selectedCoverage := 0
		for _, stuID := range studentIDs {
			if queried[stuID] {
				continue
			}
			coverage := 0
			for activityID := range activityIDsByStudent[stuID] {
				if _, ok := resolved[activityID]; !ok {
					coverage++
				}
			}
			if coverage > selectedCoverage {
				selectedStudent, selectedCoverage = stuID, coverage
			}
		}
		if selectedStudent == "" {
			break
		}
		queried[selectedStudent] = true
		activities, err := student.GetStudentActivityList(selectedStudent)
		if err != nil {
			log.Printf("活动审计暂时跳过学号 %s（活动接口不可用）: %v", selectedStudent, err)
			continue
		}
		querySuccesses++
		if len(activities) > 0 {
			studentHasConclusiveList[selectedStudent] = true
			for _, activity := range activities {
				activityID := strconv.Itoa(activity.ID)
				if _, needed := tasksByActivity[activityID]; !needed {
					continue
				}
				resolved[activityID] = schoolActivity{
					ID: activity.ID, Name: activity.Name, College: activity.Collegeview,
					StartTime: firstNonEmpty(activity.ForeachpStarttime, activity.DesignateStarttime),
					EndTime:   firstNonEmpty(activity.ForeachpEndtime, activity.DesignateEndtime),
					StartDate: activity.ForeachpStartday, EndDate: activity.ForeachpEndday,
					SignType: activity.SignType, QRCodeType: activity.QRCodeType,
				}
			}
		}
	}
	for activityID, candidateStudents := range studentsByActivity {
		if _, found := resolved[activityID]; found {
			continue
		}
		conclusive[activityID] = len(candidateStudents) > 0
		for stuID := range candidateStudents {
			if !queried[stuID] || !studentHasConclusiveList[stuID] {
				conclusive[activityID] = false
				break
			}
		}
	}

	var notices []activityAuditNotice
	for activityID, activityTasks := range tasksByActivity {
		activity, found := resolved[activityID]
		for _, task := range activityTasks {
			if !found {
				if activityDateExpired(task.ActivityEndDate, time.Now()) {
					notices = append(notices, pauseTaskForActivity(task, "expired", "活动已从平台列表移除，且任务记录的有效期已结束。")...)
					continue
				}
				if activityStartsInFuture(task.ActivityStartDate, time.Now()) {
					continue
				}
				if !conclusive[activityID] {
					log.Printf("活动 ID %s 暂无可靠查询结果，保留任务 task=%d", activityID, task.ID)
					continue
				}
				// A single list response can be temporarily incomplete. Pause first;
				// delete only after a second conclusive audit at least 12h later.
				if task.ActivityState == "missing" && task.ActivityCheckedAt != nil &&
					time.Since(*task.ActivityCheckedAt) >= 12*time.Hour {
					notices = append(notices, deleteTaskForMissingActivity(task)...)
				} else {
					notices = append(notices, pauseTaskForActivity(task, "missing", "活动已不在相关学生的活动列表中，自动任务已暂停；再次确认仍不存在后才会删除。")...)
				}
				continue
			}

			issueState, issueText := activityIssue(task, activity, time.Now())
			if err := syncActivityDetails(&task, activity); err != nil {
				log.Printf("同步活动基本信息失败: task=%d activity=%s: %v", task.ID, task.ActivityID, err)
			}
			if issueState != "" {
				notices = append(notices, pauseTaskForActivity(task, issueState, issueText)...)
				continue
			}
			resumed, err := syncNormalActivity(task, activity)
			if err != nil {
				log.Printf("同步活动信息失败: task=%d activity=%s，错误=%v", task.ID, task.ActivityID, err)
			} else if resumed {
				recovery := activityNoticesForTask(task, "活动已恢复正常，原先由系统暂停的自动任务现已恢复。")
				for i := range recovery {
					recovery[i].Recovered = true
				}
				notices = append(notices, recovery...)
			}
		}
	}

	if ready != nil {
		close(ready)
		ready = nil
	}
	for _, notice := range notices {
		var err error
		if notice.Recovered {
			err = utils.SendActivityRecoveryEmail(notice.Email, notice.StudentName, notice.StuID, notice.ActivityID, notice.ActivityName, time.Now())
		} else {
			err = utils.SendActivityIssueEmail(notice.Email, notice.StudentName, notice.StuID, notice.ActivityID, notice.ActivityName, notice.Issue, time.Now())
		}
		if err != nil {
			log.Printf("活动状态异常邮件发送失败: UserEmail=%s activity=%s: %v", notice.Email, notice.ActivityID, err)
		}
	}
	if len(tasksByActivity) > 0 && (querySuccesses == 0 || len(studentHasConclusiveList) == 0) {
		var emails []string
		if err := database.DB.Model(&database.User{}).Where("role IN ?", []int{database.RoleAdmin, database.RoleSuperAdmin}).Pluck("email", &emails).Error; err != nil {
			log.Printf("查询平台巡检通知管理员失败: %v", err)
		} else {
			for _, email := range emails {
				if err := utils.SendSystemAlertEmail(email, "未能取得任何可信的活动列表；系统保留现有任务并继续按已记录的时段调度，请检查上游平台或登录状态。", time.Now()); err != nil {
					log.Printf("发送平台巡检异常提醒失败: %v", err)
				}
			}
		}
	}
}

type schoolActivity struct {
	ID         int
	Name       string
	College    string
	StartTime  string
	EndTime    string
	StartDate  string
	EndDate    string
	SignType   int
	QRCodeType int
}

func activityIssue(task database.Task, activity schoolActivity, now time.Time) (string, string) {
	if strings.TrimSpace(activity.EndDate) == "" {
		return "date_unavailable", "平台未返回活动有效期结束日期，任务已暂停，避免按旧日期执行。"
	}
	if strings.TrimSpace(activity.EndDate) != "" {
		if _, ok := parseAuditDate(activity.EndDate); !ok {
			return "date_unavailable", "平台返回的活动结束日期无法识别，任务已暂停。"
		}
	}
	if activityDateExpired(activity.EndDate, now) {
		return "expired", "活动有效期已于 " + strings.TrimSpace(activity.EndDate) + " 结束。"
	}
	if activity.SignType != 1 || activity.QRCodeType > 0 {
		return "unsupported_mode", "当前活动不是受支持的定位签到模式：" + describeActivityMode(activity.SignType, activity.QRCodeType) + "。"
	}
	if task.SignType != activity.SignType || task.QRCodeType != activity.QRCodeType {
		return "mode_changed", "签到模式已变化。任务记录类型：" + describeActivityMode(task.SignType, task.QRCodeType) + "；平台当前类型：" + describeActivityMode(activity.SignType, activity.QRCodeType) + "。"
	}
	if student.AutomaticSignTime(activity.StartTime, activity.EndTime) == "" {
		return "time_unavailable", "平台暂未返回有效的活动起止时间，任务已暂停，避免按旧时间误执行。"
	}
	return "", ""
}

func pauseTaskForActivity(task database.Task, state, issue string) []activityAuditNotice {
	now := time.Now()
	updates := map[string]interface{}{
		"activity_state":      state,
		"activity_issue":      issue,
		"activity_checked_at": now,
	}
	// Keep the first confirmed-missing time. Repeated audits or restarts must not
	// move the 12-hour confirmation point indefinitely.
	if state == "missing" && task.ActivityState == "missing" && task.ActivityCheckedAt != nil {
		delete(updates, "activity_checked_at")
	}
	if task.ActivityState != state {
		updates["activity_auto_paused"] = task.Enabled && !task.ActivityOverride
	}
	if task.Enabled && !task.ActivityOverride {
		updates["enabled"] = false
	}
	query := database.DB.Model(&database.Task{}).Where("id = ?", task.ID)
	if task.ActivityState != state {
		query = query.Where("activity_state = ?", task.ActivityState)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		log.Printf("暂停异常活动任务失败: task=%d: %v", task.ID, result.Error)
		return nil
	}
	if task.ActivityState == state || result.RowsAffected == 0 {
		return nil
	}
	return activityNoticesForTask(task, issue)
}

func syncNormalActivity(task database.Task, activity schoolActivity) (bool, error) {
	now := time.Now()
	updates := map[string]interface{}{
		"activity_name":       activity.Name,
		"activity_college":    activity.College,
		"activity_start_date": activity.StartDate,
		"activity_end_date":   activity.EndDate,
		"activity_time_range": activityTimeRange(activity.StartTime, activity.EndTime),
		"sign_type":           activity.SignType,
		"qr_code_type":        activity.QRCodeType,
		"sign_mode":           describeActivityMode(activity.SignType, activity.QRCodeType),
		"sign_time":           student.AutomaticSignTime(activity.StartTime, activity.EndTime),
		"activity_state":      "normal",
		"activity_issue":      "",
		"activity_checked_at": now,
	}
	if task.ActivityState != "normal" {
		if task.ActivityAutoPaused && !task.ActivityOverride && student.EnsureStudentCanEnableTasks(task.StuID) == nil {
			updates["enabled"] = true
		}
		updates["activity_auto_paused"] = false
		updates["activity_override"] = false
	}
	result := database.DB.Model(&database.Task{}).Where("id = ?", task.ID).Updates(updates)
	return updates["enabled"] == true && result.RowsAffected > 0, result.Error
}

func syncActivityDetails(task *database.Task, activity schoolActivity) error {
	timeRange := activityTimeRange(activity.StartTime, activity.EndTime)
	signTime := student.AutomaticSignTime(activity.StartTime, activity.EndTime)
	if task.ActivityName == activity.Name && task.ActivityCollege == activity.College &&
		task.ActivityStartDate == activity.StartDate && task.ActivityEndDate == activity.EndDate &&
		task.ActivityTimeRange == timeRange && task.SignTime == signTime {
		return nil
	}
	updates := map[string]interface{}{
		"activity_name": activity.Name, "activity_college": activity.College,
		"activity_start_date": activity.StartDate, "activity_end_date": activity.EndDate,
		"activity_time_range": timeRange, "sign_time": signTime,
	}
	if err := database.DB.Model(&database.Task{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		return err
	}
	task.ActivityName = activity.Name
	task.ActivityCollege = activity.College
	task.ActivityStartDate = activity.StartDate
	task.ActivityEndDate = activity.EndDate
	task.ActivityTimeRange = timeRange
	task.SignTime = signTime
	return nil
}

func deleteTaskForMissingActivity(task database.Task) []activityAuditNotice {
	result := database.DB.Where("id = ? AND running_at IS NULL AND activity_state = ?", task.ID, "missing").Delete(&database.Task{})
	if result.Error != nil {
		log.Printf("删除平台已移除的活动任务失败: task=%d: %v", task.ID, result.Error)
		return nil
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return activityNoticesForTask(task, "该活动已不在此学生账号的可用活动列表中，关联签到任务已删除。")
}

func activityNoticesForTask(task database.Task, issue string) []activityAuditNotice {
	var recipients []struct {
		Email string
		Name  string
	}
	if err := database.DB.Table("user_students").Select("users.email, user_students.name").
		Joins("JOIN users ON users.id = user_students.user_id").
		Where("user_students.stu_id = ?", task.StuID).Find(&recipients).Error; err != nil {
		log.Printf("查询活动任务关联用户失败: task=%d: %v", task.ID, err)
		return nil
	}
	notices := make([]activityAuditNotice, 0, len(recipients))
	seen := make(map[string]struct{}, len(recipients))
	for _, recipient := range recipients {
		if _, ok := seen[recipient.Email]; ok {
			continue
		}
		seen[recipient.Email] = struct{}{}
		studentName := task.Name
		if studentName == "" {
			studentName = recipient.Name
		}
		notices = append(notices, activityAuditNotice{
			Email: recipient.Email, StudentName: studentName, StuID: task.StuID,
			ActivityID: task.ActivityID, ActivityName: task.ActivityName, Issue: issue,
		})
	}
	return notices
}

func activityDateExpired(value string, now time.Time) bool {
	date, ok := parseAuditDate(value)
	if !ok {
		return false
	}
	// The upstream platform serializes date-only fields as midnight timestamps
	// (for example, 2026-06-06T00:00:00). Treat midnight as an inclusive
	// calendar date, just like a plain YYYY-MM-DD value.
	if date.Hour() == 0 && date.Minute() == 0 && date.Second() == 0 && date.Nanosecond() == 0 {
		return !now.Before(date.AddDate(0, 0, 1))
	}
	return now.After(date)
}

func activityStartsInFuture(value string, now time.Time) bool {
	date, ok := parseAuditDate(value)
	return ok && now.Before(date)
}

func parseAuditDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		"2006-01-02", "2006/01/02",
		"2006-01-02 15:04:05", "2006/01/02 15:04:05",
		"2006-01-02T15:04:05", "2006/01/02T15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	// Also accept ISO-8601 values carrying a UTC offset or fractional seconds.
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func describeActivityMode(signType, qrType int) string {
	parts := make([]string, 0, 2)
	if signType == 1 {
		parts = append(parts, "定位签到")
	} else if signType > 0 {
		parts = append(parts, "平台签到类型 "+strconv.Itoa(signType))
	}
	if qrType > 0 {
		parts = append(parts, "二维码签到")
	}
	if len(parts) == 0 {
		return "未识别"
	}
	return strings.Join(parts, " · ")
}

func activityTimeRange(start, end string) string {
	if start == "" || end == "" {
		return ""
	}
	return start + "-" + end
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
