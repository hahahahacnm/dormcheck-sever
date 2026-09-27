package student

import (
	"dormcheck/database"
)

// Report whether tasks would be left without any user binding after unlinking.
func UserHasTasksForStudent(userID int, stuID string) (bool, error) {
	var count int64
	err := database.DB.Model(&database.Task{}).
		Where("stu_id = ?", stuID).
		Count(&count).Error
	if err != nil || count == 0 {
		return false, err
	}
	var otherBindings int64
	err = database.DB.Model(&database.UserStudent{}).
		Where("stu_id = ? AND user_id <> ?", stuID, userID).
		Count(&otherBindings).Error
	return otherBindings == 0, err
}
