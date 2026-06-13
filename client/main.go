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
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return nil
	}
	mode, err := common.AesDecryptCtrMode(body, aesKey)
	if err != nil {
		println("decrypt error:", err.Error())
		return nil
	}
	return mode
}
