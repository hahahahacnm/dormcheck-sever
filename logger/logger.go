package logger

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	currentLogDate string
	currentLogFile *os.File
	mu             sync.Mutex
)

// 初始化日志系统并启动轮转协程
func InitLogger() {
	// 初始写入当前日期日志
	switchLogFileToToday()

	// 每 1 分钟检查是否需要切换日志文件
	go func() {
		for {
			time.Sleep(60 * time.Second)
			rotateIfNeeded()
		}
	}()
}

// 切换到今天的日志文件
func switchLogFileToToday() {
	mu.Lock()
	defer mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today == currentLogDate {
		return // 不需要切换
	}

	// 创建日志目录（如不存在）
	logDir := "logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		_ = os.Mkdir(logDir, os.ModePerm)
	}

	// 构建日志路径
	logFilePath := filepath.Join(logDir, today+".log")

	// 打开新文件
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("日志切换失败: %v", err)
		return
	}

	// 替换输出文件
	if currentLogFile != nil {
		currentLogFile.Close()
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	currentLogDate = today
	currentLogFile = logFile
}

// 每分钟检查是否需要切换日志文件
func rotateIfNeeded() {
	today := time.Now().Format("2006-01-02")
	if today != currentLogDate {
		switchLogFileToToday()
	}
}
