package main

import (
	"configure/common"
	"net"
	"net/http"
	"os"
	"strings"
)

var (
	// rsaPriKey = []byte{}
	// rsaPubKey = []byte{}
	aesKey    = []byte("")
	whiteList = []string{}
)

func main() {
	// 设置白名单列表
	fileByte, _ := os.ReadFile("whitelist.txt")
	whiteList = strings.Split(strings.ReplaceAll(string(fileByte), "\r", ""), "\n")
	for _, str := range whiteList {
		if net.ParseIP(str) == nil {
			ips, _ := net.LookupHost(str)
			whiteList = append(whiteList, ips...)
		}
	}

	// 设置rsa密钥
	// rsaPriKey, _ = os.ReadFile("RsaPrivateKey.txt")
	// rsaPubKey, _ = os.ReadFile("RsaPublicKey.txt")
	// if len(rsaPriKey) == 0 || len(rsaPubKey) == 0 {
	// 	return
	// }

	if getenv := os.Getenv("AES_KEY"); len(getenv) == 16 || len(getenv) == 24 || len(getenv) == 32 {
		aesKey = []byte(getenv)
	}

	key, _ := os.ReadFile("./cert/cert.key")
	pem, _ := os.ReadFile("./cert/cert.pem")
	http.HandleFunc("/configFile", ReceiveHandler)
	if len(key) != 0 && len(pem) != 0 {
		println(http.ListenAndServeTLS(":6001", string(pem), string(key), nil))
	} else {
		println(http.ListenAndServe(":6001", nil))
	}
}

func ReceiveHandler(writer http.ResponseWriter, request *http.Request) {
	// 白名单校验
	isExist := false
	for _, ip := range whiteList {
		if ip == strings.Split(request.RemoteAddr, ":")[0] {
			isExist = true
			break
		}
	}
	if !isExist {
		println("RemoteAddr err -> ", request.RemoteAddr)
		return
	}

	var req = []byte(request.URL.Query().Get("fileName"))
	fileByte, err := os.ReadFile("./conf/" + string(req))
	if err != nil || len(fileByte) == 0 {
		println(err.Error())
		return
	}

	if len(aesKey) != 0 {
		// aes加密后返回
		encrypt, _ := common.AesEncryptCtrMode(fileByte, aesKey)
		_, _ = writer.Write(encrypt)
	} else {
		_, _ = writer.Write(fileByte)
	}
}
