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

// 查询单个学号的密码（仅限用户自己已绑定的学号）
func GetStudentPassword(userID int, stuID string) (string, error) {
	db := database.DB

	// 先确认该用户已经绑定了这个学号
	var bind database.UserStudent
	if err := db.Where("user_id = ? AND stu_id = ?", userID, stuID).First(&bind).Error; err != nil {
		return "", errors.New("学号未绑定或不存在")
	}

	// 查询密码
	var student database.Student
	if err := db.Where("stu_id = ?", stuID).First(&student).Error; err != nil {
		return "", errors.New("学号信息不存在")
	}

	return student.Password, nil
}
