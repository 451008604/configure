package main

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"os"
	"strings"
)

var (
	rsaPriKey = []byte{}
	rsaPubKey = []byte{}
)

func main() {
	rsaPriKey, _ = os.ReadFile("RsaPrivateKey.txt")
	rsaPubKey, _ = os.ReadFile("RsaPublicKey.txt")
	if len(rsaPriKey) == 0 || len(rsaPubKey) == 0 {
		return
	}

	http.HandleFunc("/abc", ReceiveHandler)
	println(http.ListenAndServe(":6259", nil))
}

func ReceiveHandler(writer http.ResponseWriter, request *http.Request) {
	var req = []byte(request.URL.Query().Get("text"))

	random, _ := uuid.NewRandom()
	aesKey := strings.ReplaceAll(random.String(), "-", "")[:16]
	var res = req
	// 公钥加密aesKey
	tempAesKey := RSAEncrypt([]byte(aesKey), rsaPubKey)
	res = append(res, []byte(fmt.Sprintf("\ntempAesKey -> %v\n", tempAesKey))...)

	encrypt, _ := AesCtrEncrypt(req, []byte(aesKey))
	res = append(res, []byte(fmt.Sprintf("encrypt -> %v\n", encrypt))...)

	_, _ = writer.Write(res)
}
