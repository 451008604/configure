package common

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
)

func test() {
	// // 非对称加密
	// var privateKey = []byte(``)
	// var publicKey = []byte(``)
	//
	// _ = RsaGenKey(256, "RsaPrivateKey.txt", "RsaPublicKey.txt")
	// cipherText := RSAEncrypt([]byte("测试数据"), publicKey)
	// plainText := RSADecrypt(cipherText, privateKey)
	// println(string(plainText))
	// // 对称加密。ctr模式
	// keyIv := "uWb5tp3G6i3lv2Xk"
	// encryptData, _ := AesCtrEncrypt([]byte("测试数据"), []byte(keyIv))
	// text, _ := AesCtrDecrypt(encryptData, []byte(keyIv))
	// println(string(text))
}

// 生成RSA公钥和私钥并保存在对应的目录文件下。参数bits: 指定生成的秘钥的长度, 单位: bit
func RsaGenKey(bits int, privatePath, publicPath string) error {
	// 1. 生成私钥文件
	// GenerateKey函数使用随机数据生成器random生成一对具有指定字位数的RSA密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return err
	}
	// 2. MarshalPKCS1PrivateKey将rsa私钥序列化为ASN.1 PKCS#1 DER编码
	derPrivateStream := x509.MarshalPKCS1PrivateKey(privateKey)

	// 3. Block代表PEM编码的结构, 对其进行设置
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derPrivateStream,
	}

	// 4. 创建文件
	privateFile, err := os.Create(privatePath)
	defer func(privateFile *os.File) {
		_ = privateFile.Close()
	}(privateFile)

	if err != nil {
		return err
	}

	// 5. 使用pem编码, 并将数据写入文件中
	err = pem.Encode(privateFile, &block)
	if err != nil {
		return err
	}

	// 1. 生成公钥文件
	publicKey := privateKey.PublicKey
	derPublicStream, err := x509.MarshalPKIXPublicKey(&publicKey)
	if err != nil {
		return err
	}

	block = pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: derPublicStream,
	}

	publicFile, err := os.Create(publicPath)
	defer func(publicFile *os.File) {
		_ = publicFile.Close()
	}(publicFile)

	if err != nil {
		return err
	}

	// 2. 编码公钥, 写入文件
	err = pem.Encode(publicFile, &block)
	if err != nil {
		panic(err)
		return err
	}
	return nil
}

// RSA公钥加密
func RSAEncrypt(src []byte, publicKey []byte) []byte {
	// 从数据中找出pem格式的块
	block, _ := pem.Decode(publicKey)
	if block == nil {
		return nil
	}

	// 解析一个der编码的公钥
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}

	// 公钥加密
	result, _ := rsa.EncryptPKCS1v15(rand.Reader, key.(*rsa.PublicKey), src)
	return result
}

// RSA私钥解密
func RSADecrypt(src []byte, privateKey []byte) []byte {
	// 从数据中解析出pem块
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return nil
	}

	// 解析出一个der编码的私钥
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)

	// 私钥解密
	result, err := rsa.DecryptPKCS1v15(rand.Reader, key, src)
	if err != nil {
		return nil
	}
	return result
}

// AesCtr模式对称加密
func AesEncryptCtrMode(plainText, key []byte) ([]byte, error) {
	// 创建aes对象
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// 创建分组模式
	iv := bytes.Repeat([]byte("1"), block.BlockSize())
	stream := cipher.NewCTR(block, iv)
	// 加密
	dst := make([]byte, len(plainText))
	stream.XORKeyStream(dst, plainText)

	return dst, nil
}

// AesCtr模式对称解密
func AesDecryptCtrMode(encryptData, key []byte) ([]byte, error) {
	return AesEncryptCtrMode(encryptData, key)
}
