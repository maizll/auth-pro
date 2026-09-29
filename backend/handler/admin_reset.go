package handler

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"auto_pro/config"
	"auto_pro/middleware"

	"golang.org/x/crypto/bcrypt"
)

// currentEUID 供测试替换。重设密码只允许服务器本机的 root 执行，不提供 HTTP 接口。
var currentEUID = os.Geteuid

type adminAccount struct {
	ID       uint
	Username string
}

// DispatchResetAdminPassword 处理 auth_pro reset-admin-password。
// main 必须在解析前端目录之前调用。临时目录里没有 index.html 也能重设。
// 命令在监听端口之前结束，不会把重设做成可远程调用的接口。
func DispatchResetAdminPassword(args []string) {
	if len(args) == 0 || args[0] != "reset-admin-password" {
		return
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "重设管理员密码不接受其它参数")
		os.Exit(1)
	}
	if err := requireLocalRoot(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	username, password, err := resetInstalledAdminPassword()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("管理员账号: %s\n管理员密码: %s\n登录后请在后台修改密码\n", username, password)
	os.Exit(0)
}

func requireLocalRoot() error {
	if currentEUID() != 0 {
		return errors.New("只有 root 能在服务器本机重设管理员密码")
	}
	return nil
}

func resetInstalledAdminPassword() (string, string, error) {
	db, err := config.DB()
	if err != nil {
		return "", "", fmt.Errorf("读取本站数据库失败: %w", err)
	}
	rows, err := db.Query(`SELECT id, username FROM admins ORDER BY id ASC`)
	if err != nil {
		return "", "", fmt.Errorf("读取管理员失败: %w", err)
	}
	defer rows.Close()
	var accounts []adminAccount
	for rows.Next() {
		var item adminAccount
		if err := rows.Scan(&item.ID, &item.Username); err != nil {
			return "", "", fmt.Errorf("读取管理员失败: %w", err)
		}
		accounts = append(accounts, item)
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("读取管理员失败: %w", err)
	}
	target, err := chooseAdminPasswordTarget(accounts)
	if err != nil {
		return "", "", err
	}
	password, err := randomDigitPassword(8)
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", "", errors.New("密码加密失败")
	}
	if err := middleware.EnsurePasswordChangedAtColumn(db, "admins"); err != nil {
		return "", "", fmt.Errorf("准备密码变更时间失败: %w", err)
	}
	result, err := db.Exec(
		`UPDATE admins SET password_hash = ?, password_changed_at = ? WHERE id = ?`,
		string(hash), middleware.NewPasswordChangeStamp(), target.ID,
	)
	if err != nil {
		return "", "", fmt.Errorf("写入新密码失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", "", fmt.Errorf("确认新密码失败: %w", err)
	}
	if affected != 1 {
		return "", "", errors.New("新密码没有写入数据库，已停止")
	}
	var stored string
	if err := db.QueryRow(`SELECT password_hash FROM admins WHERE id = ?`, target.ID).Scan(&stored); err != nil {
		return "", "", fmt.Errorf("回读新密码失败: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)); err != nil {
		return "", "", errors.New("新密码与库里的哈希不一致，已停止")
	}
	return target.Username, password, nil
}

// chooseAdminPasswordTarget 优先重设安装时创建的 admin。
// 没有这个用户名时，只有库里恰好一个管理员才改；多个账号时拒绝，避免改错人。
func chooseAdminPasswordTarget(rows []adminAccount) (adminAccount, error) {
	if len(rows) == 0 {
		return adminAccount{}, errors.New("没有管理员账号，无法重设密码")
	}
	var named []adminAccount
	for _, row := range rows {
		if row.Username == "admin" {
			named = append(named, row)
		}
	}
	if len(named) == 1 {
		return named[0], nil
	}
	if len(named) > 1 {
		return adminAccount{}, errors.New("存在多个用户名为 admin 的管理员，已拒绝修改")
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	return adminAccount{}, errors.New("没有用户名为 admin 的管理员，且存在多个管理员，已拒绝修改")
}

func randomDigitPassword(length int) (string, error) {
	if length < 1 || length > 32 {
		return "", errors.New("密码长度不正确")
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成密码失败: %w", err)
	}
	for i, b := range buf {
		buf[i] = '0' + (b % 10)
	}
	return string(buf), nil
}
