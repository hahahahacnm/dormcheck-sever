package user

import (
	"dormcheck/database"
	"errors"
)

// 解绑学号前检查是否还有任务，若无任务才解绑
func UnbindStudent(userID int, stuID string) error {
	db := database.DB

	// 查询该用户该学号的任务数量
	var taskCount int64
	err := db.Model(&database.Task{}).
		Where("user_id = ? AND stu_id = ?", userID, stuID).
		Count(&taskCount).Error
	if err != nil {
		return err
	}

	if taskCount > 0 {
		return errors.New("该学号还有任务未删除，请先删除任务再解绑")
	}

	// 删除绑定关系
	result := db.Where("user_id = ? AND stu_id = ?", userID, stuID).Delete(&database.UserStudent{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("绑定关系不存在")
	}

	return nil
}

// 查询用户已绑定的学号列表
func GetBoundStudents(userID int) ([]database.UserStudent, error) {
	var binds []database.UserStudent
	err := database.DB.Where("user_id = ?", userID).Find(&binds).Error
	return binds, err
}
