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

var (
	aesKey    = []byte("")
	whiteList []string
	wlMu      sync.RWMutex
	lastWlMod time.Time
	logger    *log.Logger
)

const logMaxLines = 300
const logTrimLines = 200

type logRotator struct {
	mu   sync.Mutex
	file *os.File
}

func (r *logRotator) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n, err = r.file.Write(p)
	if err != nil {
		return n, err
	}

	info, err := r.file.Stat()
	if err != nil || info.Size() == 0 {
		return n, nil
	}

	_, err = r.file.Seek(0, 0)
	if err != nil {
		return n, nil
	}

	scanner := bufio.NewScanner(r.file)
	lines := 0
	for scanner.Scan() {
		lines++
	}

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

func main() {
	initLogger()

	if getenv := os.Getenv("AES_KEY"); len(getenv) == 16 || len(getenv) == 24 || len(getenv) == 32 {
		aesKey = []byte(getenv)
		log.Printf("AES enabled with key length %d", len(aesKey))
	} else if getenv := os.Getenv("AES_KEY"); getenv != "" {
		log.Printf("WARNING: AES_KEY length %d is invalid, must be 16/24/32. Falling back to plaintext.", len(getenv))
	}

	common.InitRedis()
	_ = common.GetConfigManager().Load()
	go watchConfigChanges()
	reloadWhitelist()

	key, _ := os.ReadFile("./cert/cert.key")
	pem, _ := os.ReadFile("./cert/cert.pem")

	http.HandleFunc("/configFile", logRequest(ReceiveHandler))
	http.HandleFunc("/customConfig", logRequest(CustomConfigHandler))

	if len(key) != 0 && len(pem) != 0 {
		log.Println("Starting HTTPS server on :6001")
		log.Fatal(http.ListenAndServeTLS(":6001", string(pem), string(key), nil))
	} else {
		log.Println("Starting HTTP server on :6001")
		log.Fatal(http.ListenAndServe(":6001", nil))
	}
}

func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := extractIP(r.RemoteAddr)
		log.Printf("[REQUEST] %s %s from %s", r.Method, r.URL.String(), ip)
		
		writer := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler(writer, r)
		
		log.Printf("[RESPONSE] %s %s from %s - status: %d, duration: %v", r.Method, r.URL.String(), ip, writer.statusCode, time.Since(start))
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

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

func reloadWhitelist() {
	fileInfo, err := os.Stat("whitelist.txt")
	if err != nil {
		log.Printf("whitelist.txt not found: %v", err)
		return
	}
	modTime := fileInfo.ModTime()
	wlMu.RLock()
	cached := lastWlMod
	wlMu.RUnlock()

	if !modTime.After(cached) {
		return
	}

	fileByte, err := os.ReadFile("whitelist.txt")
	if err != nil {
		log.Printf("failed to read whitelist.txt: %v", err)
		return
	}

	lines := strings.Split(strings.ReplaceAll(string(fileByte), "\r", "\n"), "\n")
	newList := make([]string, 0, len(lines))
	for _, str := range lines {
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}
		newList = append(newList, str)
		if net.ParseIP(str) == nil {
			ips, err := net.LookupHost(str)
			if err != nil {
				log.Printf("failed to resolve %s: %v", str, err)
				continue
			}
			newList = append(newList, ips...)
		}
	}

	wlMu.Lock()
	whiteList = newList
	lastWlMod = modTime
	wlMu.Unlock()
	log.Printf("whitelist reloaded: %d entries", len(newList))
}

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

	fileByte, err := os.ReadFile("./conf/" + fileName)
	if err != nil {
		log.Printf("failed to read file %s: %v", fileName, err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

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
		data, err = common.GetConfigManager().GetConfig(configName)
		if err != nil {
			log.Printf("failed to get config %s: %v", configName, err)
			http.Error(w, "Config not found", http.StatusNotFound)
			return
		}
	} else {
		ip := extractIP(r.RemoteAddr)
		data, err = common.GetConfigManager().GetMergedConfig(ip)
		if err != nil {
			log.Printf("failed to get merged config for %s: %v", ip, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

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

func watchConfigChanges() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("failed to create watcher: %v", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add("conf"); err != nil {
		log.Printf("failed to watch conf: %v", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(event.Name, ".json") {
				continue
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				log.Printf("config file changed: %s", event.Name)
				if err := common.GetConfigManager().Load(); err != nil {
					log.Printf("failed to reload config: %v", err)
					continue
				}
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
