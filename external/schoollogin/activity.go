package schoollogin

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Activity 结构体定义示例
type Activity struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	ForeachpStarttime  string `json:"foreachp_starttime"`
	ForeachpEndtime    string `json:"foreachp_endtime"`
	DesignateStarttime string `json:"designate_strattime"`
	DesignateEndtime   string `json:"designate_endtime"`
	Collegeview        string `json:"collegeview"`
	Sigintaskview      string `json:"sigintaskview"`
	ForeachpStartday   string `json:"foreachp_startday"`
	ForeachpEndday     string `json:"foreachp_endday"`
	SignType           int    `json:"sigintype"`
	QRCodeType         int    `json:"qrcodetype"`
}

func GetActivityList(cookies []*http.Cookie) ([]Activity, error) {
	url := ActivityListURL

	// 构造请求
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头和Cookie
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", PlatBaseURL)
	req.Header.Set("Referer", PlatBaseURL+"/")
	var cookieStr string
	for _, ck := range cookies {
		cookieStr += ck.Name + "=" + ck.Value + "; "
	}
	req.Header.Set("Cookie", cookieStr)

	client := newSchoolClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	body, err := readPlatformResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 假设响应是 JSON 格式，里面有 data 字段是活动列表
	var respData struct {
		Data json.RawMessage `json:"data"`
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
	}

	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, fmt.Errorf("解析活动列表失败: %v", err)
	}

	if len(respData.Data) == 0 || string(respData.Data) == "null" {
		return nil, fmt.Errorf("活动接口未返回有效列表（code=%d，msg=%s）", respData.Code, respData.Msg)
	}
	var activities []Activity
	if err := json.Unmarshal(respData.Data, &activities); err != nil {
		return nil, fmt.Errorf("活动列表格式无效: %w", err)
	}
	return activities, nil
}
