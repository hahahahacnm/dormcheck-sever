package user

import (
	"dormcheck/database"
	"dormcheck/utils"
	"encoding/base64"
	"errors"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 解码 Base64 密码
func decodePassword(encodedPwd string) (string, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedPwd)
	if err != nil {
		return "", errors.New("密码解码失败")
	}
	return string(decodedBytes), nil
}

// 注册用户
func Register(username, email, encodedPassword, code string) error {
	if username == "" || email == "" || encodedPassword == "" || code == "" {
		return errors.New("用户名、邮箱、密码和验证码不能为空")
	}

	password, err := decodePassword(encodedPassword)
	if err != nil {
		return err
	}

	db := database.DB

	// 校验验证码
	var evc database.EmailVerificationCode
	if err := db.Where("email = ? AND code = ? AND purpose = ? AND expires_at > ?", email, code, "register", time.Now()).First(&evc).Error; err != nil {
		return errors.New("验证码错误或已过期")
	}

	// 查重：用户名或邮箱任意一个已存在都返回错误
	var existing database.User
	if err := db.Where("username = ? OR email = ?", username, email).First(&existing).Error; err == nil {
		return errors.New("用户名或邮箱已存在")
	}

	hashed, err := utils.HashPassword(password)
	if err != nil {
		return err
	}

	newUser := database.User{
		Username:      username,
		Email:         email,
		EmailVerified: true,
		Password:      hashed,
		TokenVersion:  1,
		Role:          1,
	}

	if err := db.Create(&newUser).Error; err != nil {
		return err
	}

	_ = db.Delete(&evc).Error

	return nil
}

// 发送注册验证码
func SendRegisterVerificationCode(email string) error {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(email) {
		return errors.New("邮箱格式不正确")
	}

	db := database.DB

	var lastCode database.EmailVerificationCode
	err := db.Where("email = ? AND purpose = ?", email, "register").
		Order("created_at DESC").First(&lastCode).Error
	if err == nil && time.Since(lastCode.CreatedAt) < 1*time.Minute {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}

	code, err := utils.GenerateVerificationCode()
	if err != nil {
		return err
	}

	if err := utils.SendVerificationCodeEmail(email, code); err != nil {
		return err
	}

	evc := database.EmailVerificationCode{
		Email:     email,
		Code:      code,
		Purpose:   "register",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		CreatedAt: time.Now(),
	}

	return db.Create(&evc).Error
}

// 登录，返回 token
func Login(identifier, encodedPassword string) (string, error) {
	if identifier == "" || encodedPassword == "" {
		return "", errors.New("用户名/邮箱 和 密码不能为空")
	}

	password, err := decodePassword(encodedPassword)
	if err != nil {
		return "", err
	}

	var user database.User
	db := database.DB

	if err := db.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error; err != nil {
		return "", errors.New("用户名或密码错误")
	}

	if !utils.CheckPassword(user.Password, password) {
		return "", errors.New("用户名或密码错误")
	}

	return utils.GenerateToken(user)
}

// 强制注销所有设备（让所有 token 失效）
func ForceLogoutAll(userID int) error {
	result := database.DB.Model(&database.User{}).Where("id = ?", userID).
		Update("token_version", gorm.Expr("token_version + 1"))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	return nil
}

// 通过ID查询用户
func GetUserByID(userID int) (*database.User, error) {
	var u database.User
	if err := database.DB.First(&u, userID).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// 校验密码哈希
func CheckPasswordHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func UpdateUserPassword(userID int, newHash string) error {
	return database.DB.Model(&database.User{}).Where("id = ?", userID).Update("password", newHash).Error
}

// ------------- 修改密码，且限制新密码不能和旧密码相同 -------------
func ChangeUserPassword(userID int, encodedOldPassword, encodedNewPassword string) error {
	oldPwd, err := decodePassword(encodedOldPassword)
	if err != nil {
		return err
	}
	newPwd, err := decodePassword(encodedNewPassword)
	if err != nil {
		return err
	}

	db := database.DB

	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}

	if !CheckPasswordHash(oldPwd, user.Password) {
		return errors.New("旧密码错误")
	}

	// 用 bcrypt 校验新密码是否和旧密码相同
	if CheckPasswordHash(newPwd, user.Password) {
		return errors.New("新密码不能与旧密码相同")
	}

	newHashedPwd, err := HashPassword(newPwd)
	if err != nil {
		return err
	}

	if err := UpdateUserPassword(userID, newHashedPwd); err != nil {
		return errors.New("更新密码失败")
	}

	return nil
}

// ------------- 修改邮箱，限制新旧邮箱不能相同 -------------
func ChangeUserEmail(userID int, newEmail, code string) error {
	if newEmail == "" || code == "" {
		return errors.New("新邮箱和验证码不能为空")
	}

	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(newEmail) {
		return errors.New("邮箱格式不正确")
	}

	db := database.DB

	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}

	if user.Email == newEmail {
		return errors.New("新邮箱不能与旧邮箱相同")
	}

	var evc database.EmailVerificationCode
	if err := db.Where("email = ? AND code = ? AND purpose = ? AND expires_at > ?", newEmail, code, "change_email", time.Now()).
		First(&evc).Error; err != nil {
		return errors.New("验证码错误或已过期")
	}

	var existing database.User
	if err := db.Where("email = ?", newEmail).First(&existing).Error; err == nil && existing.ID != userID {
		return errors.New("该邮箱已被其他用户绑定")
	}

	if err := db.Model(&database.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"email":          newEmail,
		"email_verified": false,
	}).Error; err != nil {
		return errors.New("更新邮箱失败")
	}

	_ = db.Delete(&evc).Error

	return nil
}

func SendChangeEmailVerificationCode(email string) error {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(email) {
		return errors.New("邮箱格式不正确")
	}

	db := database.DB

	var lastCode database.EmailVerificationCode
	err := db.Where("email = ? AND purpose = ?", email, "change_email").
		Order("created_at DESC").First(&lastCode).Error
	if err == nil && time.Since(lastCode.CreatedAt) < 1*time.Minute {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}

	code, err := utils.GenerateVerificationCode()
	if err != nil {
		return err
	}

	if err := utils.SendVerificationCodeEmail(email, code); err != nil {
		return err
	}

	evc := database.EmailVerificationCode{
		Email:     email,
		Code:      code,
		Purpose:   "change_email",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		CreatedAt: time.Now(),
	}

	return db.Create(&evc).Error
}

// 赞助码激活
func UseSponsorCode(userID int, code string) error {
	db := database.DB

	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if user.Role == 2 {
		return errors.New("您已是赞助用户，无需重复激活")
	}

	var sac database.SponsorActivationCode
	err := db.Where("code = ? AND used = ?", code, false).First(&sac).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("激活码无效或已被使用")
		}
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		if err := tx.Model(&database.SponsorActivationCode{}).Where("id = ?", sac.ID).Updates(map[string]interface{}{
			"used":    true,
			"used_by": userID,
			"used_at": now,
		}).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.User{}).Where("id = ?", userID).Update("role", 2).Error; err != nil {
			return err
		}

		return nil
	})
}

// 忘记密码发送验证码
func SendResetPasswordCode(email string) error {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(email) {
		return errors.New("邮箱格式不正确")
	}

	db := database.DB

	var user database.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return errors.New("邮箱不存在")
	}

	var lastCode database.EmailVerificationCode
	err := db.Where("email = ? AND purpose = ?", email, "reset").
		Order("created_at DESC").First(&lastCode).Error
	if err == nil && time.Since(lastCode.CreatedAt) < time.Minute {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}

	code, err := utils.GenerateVerificationCode()
	if err != nil {
		return err
	}

	if err := utils.SendVerificationCodeEmail(email, code); err != nil {
		return err
	}

	evc := database.EmailVerificationCode{
		Email:     email,
		Code:      code,
		Purpose:   "reset",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		CreatedAt: time.Now(),
	}

	return db.Create(&evc).Error
}

// 重置密码通过验证码
func ResetPasswordByCode(email, code, encodedNewPassword string) error {
	if email == "" || code == "" || encodedNewPassword == "" {
		return errors.New("邮箱、验证码和新密码不能为空")
	}

	db := database.DB

	var evc database.EmailVerificationCode
	if err := db.Where("email = ? AND code = ? AND purpose = ? AND expires_at > ?", email, code, "reset", time.Now()).
		First(&evc).Error; err != nil {
		return errors.New("验证码错误或已过期")
	}

	newPassword, err := decodePassword(encodedNewPassword)
	if err != nil {
		return err
	}

	hashedPwd, err := utils.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := db.Model(&database.User{}).Where("email = ?", email).Update("password", hashedPwd).Error; err != nil {
		return errors.New("密码更新失败")
	}

	_ = db.Delete(&evc).Error

	return nil
}

// 重置密码并强制注销所有设备
func ResetPasswordAndForceLogout(email, code, encodedNewPassword string) error {
	if err := ResetPasswordByCode(email, code, encodedNewPassword); err != nil {
		return err
	}

	var user database.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return errors.New("用户不存在")
	}

	_ = ForceLogoutAll(user.ID)

	return nil
}
