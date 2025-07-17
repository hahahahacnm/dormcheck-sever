package student

import (
	"dormcheck/database"
)

// 判断用户是否对某个学号有绑定的任务
func UserHasTasksForStudent(userID int, stuID string) (bool, error) {
	var count int64
	err := database.DB.Model(&database.Task{}).
		Where("user_id = ? AND stu_id = ?", userID, stuID).
		Count(&count).Error
	return count > 0, err
}
