package main

import (
	"configure/common"
	"io"
	"net/http"
)

// aesKey 客户端 AES 密钥，用于解密服务器返回的加密内容
// 必须与服务器配置的 AES_KEY 环境变量一致
// 生产环境中应通过安全方式获取，避免硬编码
var aesKey = []byte("qzqlkjiiosdflknx")

// main 客户端示例入口
// 演示如何获取远程配置文件并解密
func main() {
	configData := GetRemoteConfigData("config.toml")
	println(string(configData))
}

// GetRemoteConfigData 从配置服务器获取文件内容
// 流程：
// 1. 发送 HTTP GET 请求到服务器的 /configFile 端点
// 2. 读取响应体
// 3. 使用 AES-CTR 模式解密（自动提取 IV）
// 参数：
//   fileName - 配置文件名（如 config.toml）
// 返回：
//   解密后的原始内容，如果失败返回 nil
// 注意：
// - 如果服务器未启用 AES 加密，返回原始内容
// - 如果解密失败，打印错误信息并返回 nil
func GetRemoteConfigData(fileName string) []byte {
	resp, err := http.Get("http://127.0.0.1:6001/configFile?fileName=" + fileName)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return nil
	}
	// 解密响应内容
	// 自动处理 IV 前缀（服务器加密时自动添加）
	mode, err := common.AesDecryptCtrMode(body, aesKey)
	if err != nil {
		println("decrypt error:", err.Error())
		return nil
	}
	return mode
}
