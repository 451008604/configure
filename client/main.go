package main

import (
	"configure/common"
	"io"
	"net/http"
)

var aesKey = []byte("qzqlkjiiosdflknx")

func main() {
	configData := GetRemoteConfigData("config.toml")
	println(string(configData))
}

func GetRemoteConfigData(fileName string) []byte {
	resp, err := http.Get("http://127.0.0.1:6001/configFile?fileName=" + fileName)
	if err != nil {
		return nil
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// 读取并打印响应主体
	if body, _ := io.ReadAll(resp.Body); len(body) != 0 {
		mode, _ := common.AesDecryptCtrMode(body, aesKey)
		return mode
	}
	return nil
}
