package schoollogin

import (
	"dormcheck/config"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SimpleCookie struct {
	Name  string
	Value string
}

const (
	PlatBaseURL     = "https://plat.swmu.edu.cn"
	MeBaseURL       = "https://me.swmu.edu.cn"
	CaptchaURL      = PlatBaseURL + "/Authentication/GetValidateCode"
	LoginURL        = PlatBaseURL + "/MyAuthentication/put/"
	ActivityListURL = PlatBaseURL + "/studentwork/PunchMStudent/GetActivityList"
	SubmitSigninURL = PlatBaseURL + "/studentwork/PunchMStudent/SubmitSignin"
)

// ErrSessionExpired indicates that the school platform redirected an API request
// to its SSO sign-in page because the saved micro-platform session is no longer valid.
var ErrSessionExpired = errors.New("微学工会话已失效（SSO 统一身份认证），请重新登录并绑定学号")

func newSchoolClient() *http.Client {
	seconds := config.GetInt("school_request_timeout_seconds", 30)
	if seconds < 5 {
		seconds = 5
	}
	if seconds > 120 {
		seconds = 120
	}
	return &http.Client{Timeout: time.Duration(seconds) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// NewSchoolClient returns a client that exposes redirects for explicit SSO handling.
func NewSchoolClient() *http.Client { return newSchoolClient() }

func checkPlatformResponse(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("%w（HTTP %d，跳转至 %s）", ErrSessionExpired, resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("微学工接口返回 HTTP %d", resp.StatusCode)
	}
	content := strings.ToLower(string(body))
	if strings.Contains(content, "tysfrz-login") || (strings.Contains(content, "id=\"fm1\"") && strings.Contains(content, "execution")) {
		return ErrSessionExpired
	}
	return nil
}

func readPlatformResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("读取微学工响应失败: %w", err)
	}
	if len(body) > 4<<20 {
		return nil, errors.New("微学工响应过大")
	}
	if err := checkPlatformResponse(resp, body); err != nil {
		return nil, err
	}
	return body, nil
}

// ReadPlatformResponse reads an API response and returns ErrSessionExpired for SSO pages.
func ReadPlatformResponse(resp *http.Response) ([]byte, error) { return readPlatformResponse(resp) }
