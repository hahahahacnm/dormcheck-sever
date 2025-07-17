package student

import (
	"dormcheck/database"
	"dormcheck/external/schoollogin"
	"dormcheck/utils"
	"fmt"
)

// GetStudentActivityList 查询某个学号的可签到活动
func GetStudentActivityList(stuID string) ([]schoollogin.Activity, error) {
	// 查数据库获取 cookies
	student, err := database.GetStudentByStuID(stuID)
	if err != nil {
		return nil, fmt.Errorf("未找到学号信息: %v", err)
	}

	cookies, err := utils.DeserializeCookies(student.Cookies)
	if err != nil {
		return nil, fmt.Errorf("cookie 解析失败: %v", err)
	}

	// 发起请求
	activities, err := schoollogin.GetActivityList(cookies)
	if err != nil {
		return nil, fmt.Errorf("获取活动失败: %v", err)
	}

	return activities, nil
}
