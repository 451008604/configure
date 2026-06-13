package main

import (
	"bufio"
	"bytes"
	"configure/common"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// 全局变量定义
var (
	aesKey    = []byte("")   // AES加密密钥，从环境变量 AES_KEY 读取
	whiteList []string       // IP白名单列表
	wlMu      sync.RWMutex   // 白名单读写锁，保证并发安全
	lastWlMod time.Time      // 白名单文件最后修改时间，用于缓存判断
	logger    *log.Logger    // 自定义日志记录器
)

// 日志轮转配置
const logMaxLines = 300   // 日志文件最大行数
const logTrimLines = 200  // 超过最大行数时删除的行数

// logRotator 自定义日志写入器，支持自动轮转
// 当日志行数超过 logMaxLines 时，自动删除前 logTrimLines 行
// 防止日志文件无限增长
type logRotator struct {
	mu   sync.Mutex
	file *os.File
}

// Write 实现 io.Writer 接口
// 每次写入后检查日志行数，超过阈值则自动清理旧日志
func (r *logRotator) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n, err = r.file.Write(p)
	if err != nil {
		return n, err
	}

	// 获取文件大小，为空则跳过检查
	info, err := r.file.Stat()
	if err != nil || info.Size() == 0 {
		return n, nil
	}

	// 回到文件开头准备统计行数
	_, err = r.file.Seek(0, 0)
	if err != nil {
		return n, nil
	}

	// 扫描统计当前行数
	scanner := bufio.NewScanner(r.file)
	lines := 0
	for scanner.Scan() {
		lines++
	}

	// 如果超过最大行数，删除前 logTrimLines 行
	if lines > logMaxLines {
		_, _ = r.file.Seek(0, 0)
		scanner := bufio.NewScanner(r.file)
		skip := logTrimLines
		var buf bytes.Buffer
		for scanner.Scan() {
			if skip > 0 {
				skip--
				continue
			}
			buf.Write(scanner.Bytes())
			buf.WriteByte('\n')
		}

		// 关闭旧文件，重写新内容
		name := r.file.Name()
		_ = r.file.Close()
		newFile, err := os.OpenFile(name, os.O_TRUNC|os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return n, err
		}
		_, _ = newFile.Write(buf.Bytes())
		r.file = newFile
	}

	return n, nil
}

// initLogger 初始化日志系统
// 日志写入 configure.log 文件，格式包含时间戳
func initLogger() {
	logFile, err := os.OpenFile("configure.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	rotator := &logRotator{file: logFile}
	logger = log.New(rotator, "", log.LstdFlags)
	log.SetOutput(rotator)
	log.SetFlags(log.LstdFlags)
}

// main 程序入口函数
// 初始化流程：
// 1. 初始化日志系统
// 2. 加载 AES 密钥
// 3. 连接 Redis
// 4. 加载配置文件
// 5. 启动文件监听
// 6. 加载白名单
// 7. 启动 HTTP/HTTPS 服务
func main() {
	initLogger()

	// 从环境变量加载 AES 密钥
	// 支持 16/24/32 字节长度（对应 AES-128/192/256）
	if getenv := os.Getenv("AES_KEY"); len(getenv) == 16 || len(getenv) == 24 || len(getenv) == 32 {
		aesKey = []byte(getenv)
		log.Printf("AES enabled with key length %d", len(aesKey))
	} else if getenv := os.Getenv("AES_KEY"); getenv != "" {
		log.Printf("WARNING: AES_KEY length %d is invalid, must be 16/24/32. Falling back to plaintext.", len(getenv))
	}

	// 初始化 Redis 连接
	common.InitRedis()

	// 加载配置文件
	_ = common.GetConfigManager().Load()

	// 启动配置文件监听，异步监控文件变化
	go watchConfigChanges()

	// 加载白名单
	reloadWhitelist()

	// 检查是否存在 TLS 证书
	key, _ := os.ReadFile("./cert/cert.key")
	pem, _ := os.ReadFile("./cert/cert.pem")

	// 注册 HTTP 路由，使用 logRequest 包装以记录请求日志
	http.HandleFunc("/configFile", logRequest(ReceiveHandler))
	http.HandleFunc("/customConfig", logRequest(CustomConfigHandler))

	// 启动 HTTPS 或 HTTP 服务
	if len(key) != 0 && len(pem) != 0 {
		log.Println("Starting HTTPS server on :6001")
		log.Fatal(http.ListenAndServeTLS(":6001", string(pem), string(key), nil))
	} else {
		log.Println("Starting HTTP server on :6001")
		log.Fatal(http.ListenAndServe(":6001", nil))
	}
}

// logRequest HTTP 请求日志中间件
// 记录每个请求的来源 IP、方法、URL、状态码和处理耗时
func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := extractIP(r.RemoteAddr)
		log.Printf("[REQUEST] %s %s from %s", r.Method, r.URL.String(), ip)
		
		// 使用自定义 responseWriter 捕获状态码
		writer := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler(writer, r)
		
		log.Printf("[RESPONSE] %s %s from %s - status: %d, duration: %v", r.Method, r.URL.String(), ip, writer.statusCode, time.Since(start))
	}
}

// responseWriter 自定义响应写入器
// 用于拦截 WriteHeader 调用以获取 HTTP 状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader 重写 WriteHeader 方法，记录状态码
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// extractIP 从 RemoteAddr 中提取 IP 地址
// 支持 IPv4 和 IPv6 格式（如 [::1]:6001）
// 处理流程：
// 1. 尝试使用 net.SplitHostPort 分离 IP 和端口
// 2. 失败时尝试直接解析 IP
// 3. 仍失败则返回原始字符串
func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		if ip := net.ParseIP(remoteAddr); ip != nil {
			return ip.String()
		}
		return remoteAddr
	}
	return host
}

// reloadWhitelist 从 whitelist.txt 加载白名单
// 使用 mtime 缓存策略，仅当文件修改时间变化时才重新加载
// 支持域名解析为 IP 地址
func reloadWhitelist() {
	fileInfo, err := os.Stat("whitelist.txt")
	if err != nil {
		log.Printf("whitelist.txt not found: %v", err)
		return
	}
	modTime := fileInfo.ModTime()

	// 读取缓存的修改时间
	wlMu.RLock()
	cached := lastWlMod
	wlMu.RUnlock()

	// 如果文件未修改，跳过加载
	if !modTime.After(cached) {
		return
	}

	fileByte, err := os.ReadFile("whitelist.txt")
	if err != nil {
		log.Printf("failed to read whitelist.txt: %v", err)
		return
	}

	// 解析白名单内容
	lines := strings.Split(strings.ReplaceAll(string(fileByte), "\r", "\n"), "\n")
	newList := make([]string, 0, len(lines))
	for _, str := range lines {
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}
		newList = append(newList, str)
		// 如果不是 IP 地址，尝试解析域名
		if net.ParseIP(str) == nil {
			ips, err := net.LookupHost(str)
			if err != nil {
				log.Printf("failed to resolve %s: %v", str, err)
				continue
			}
			newList = append(newList, ips...)
		}
	}

	// 更新白名单
	wlMu.Lock()
	whiteList = newList
	lastWlMod = modTime
	wlMu.Unlock()
	log.Printf("whitelist reloaded: %d entries", len(newList))
}

// checkWhitelist 检查请求 IP 是否在白名单中
// 先调用 reloadWhitelist 检查是否需要更新，再遍历白名单列表
func checkWhitelist(remoteAddr string) bool {
	reloadWhitelist()
	ip := extractIP(remoteAddr)
	wlMu.RLock()
	defer wlMu.RUnlock()
	for _, allowed := range whiteList {
		if allowed == ip {
			return true
		}
	}
	return false
}

// ReceiveHandler 处理 /configFile 请求
// 返回 conf/json/ 或 conf/yml/ 目录下的原始文件内容
// 如果设置了 AES 密钥，则返回加密后的内容
func ReceiveHandler(w http.ResponseWriter, r *http.Request) {
	if !checkWhitelist(r.RemoteAddr) {
		log.Printf("RemoteAddr blocked: %s", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	fileName := r.URL.Query().Get("fileName")
	if fileName == "" {
		http.Error(w, "fileName required", http.StatusBadRequest)
		return
	}

	// 先查找 json 目录，不存在则查找 yml 目录
	filePath := "./conf/json/" + fileName
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		filePath = "./conf/yml/" + fileName
	}
	fileByte, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("failed to read file %s: %v", fileName, err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// 如果设置了 AES 密钥，加密后返回
	if len(aesKey) != 0 {
		encrypt, err := common.AesEncryptCtrMode(fileByte, aesKey)
		if err != nil {
			log.Printf("AES encrypt failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(encrypt)
	} else {
		_, _ = w.Write(fileByte)
	}
}

// CustomConfigHandler 处理 /customConfig 请求
// 支持两种模式：
// 1. 指定配置名：?config=sub1 返回对应配置的合并结果
// 2. 默认模式：根据请求 IP 返回 base.json + overrides.json 的合并结果
// 所有结果以 JSON 格式返回，支持 AES 加密
func CustomConfigHandler(w http.ResponseWriter, r *http.Request) {
	if !checkWhitelist(r.RemoteAddr) {
		log.Printf("RemoteAddr blocked: %s", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var data []byte
	var err error

	configName := r.URL.Query().Get("config")
	if configName != "" {
		// 指定配置名模式
		data, err = common.GetConfigManager().GetConfig(configName)
		if err != nil {
			log.Printf("failed to get config %s: %v", configName, err)
			http.Error(w, "Config not found", http.StatusNotFound)
			return
		}
	} else {
		// 默认 IP 模式：base.json + IP 覆盖
		ip := extractIP(r.RemoteAddr)
		data, err = common.GetConfigManager().GetMergedConfig(ip)
		if err != nil {
			log.Printf("failed to get merged config for %s: %v", ip, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// 如果设置了 AES 密钥，加密后返回
	if len(aesKey) != 0 {
		encrypt, err := common.AesEncryptCtrMode(data, aesKey)
		if err != nil {
			log.Printf("AES encrypt failed: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(encrypt)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
	}
}

// watchConfigChanges 使用 fsnotify 监听配置文件变化
// 监听目录：conf/json/ 和 conf/yml/
// 当任何 .json 或 .yml 文件修改时：
// 1. 重新加载所有配置
// 2. 向 Redis 发布 config_updates 通知
func watchConfigChanges() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("failed to create watcher: %v", err)
		return
	}
	defer watcher.Close()

	// 监听 JSON 配置目录
	if err := watcher.Add("conf/json"); err != nil {
		log.Printf("failed to watch conf/json: %v", err)
		return
	}
	// 监听 YAML 配置目录
	if err := watcher.Add("conf/yml"); err != nil {
		log.Printf("failed to watch conf/yml: %v", err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// 只处理配置文件变更
			if !strings.HasSuffix(event.Name, ".json") && !strings.HasSuffix(event.Name, ".yml") && !strings.HasSuffix(event.Name, ".yaml") {
				continue
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				log.Printf("config file changed: %s", event.Name)
				// 重新加载配置
				if err := common.GetConfigManager().Load(); err != nil {
					log.Printf("failed to reload config: %v", err)
					continue
				}
				// 发送 Redis 通知
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				fileName := filepath.Base(event.Name)
				msg := fmt.Sprintf(`{"event":"config_changed","file":"%s"}`, fileName)
				log.Printf("[REDIS] Publishing config change: %s", msg)
				if err := common.PublishConfigChange(ctx, "config_updates", msg); err != nil {
					log.Printf("[REDIS] Failed to publish config change: %v", err)
				}
				cancel()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}
