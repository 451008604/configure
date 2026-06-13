package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

func RsaGenKey(bits int, privatePath, publicPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return err
	}

	derPrivateStream := x509.MarshalPKCS1PrivateKey(privateKey)
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derPrivateStream,
	}

	privateFile, err := os.Create(privatePath)
	if err != nil {
		return err
	}
	defer privateFile.Close()

	if err := pem.Encode(privateFile, &block); err != nil {
		return err
	}

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
	if err != nil {
		return err
	}
	defer publicFile.Close()

	if err := pem.Encode(publicFile, &block); err != nil {
		return err
	}
	return nil
}

func RSAEncrypt(src []byte, publicKey []byte) []byte {
	block, _ := pem.Decode(publicKey)
	if block == nil {
		return nil
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}

	result, _ := rsa.EncryptPKCS1v15(rand.Reader, key.(*rsa.PublicKey), src)
	return result
}

func RSADecrypt(src []byte, privateKey []byte) []byte {
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil
	}

	result, err := rsa.DecryptPKCS1v15(rand.Reader, key, src)
	if err != nil {
		return nil
	}
	return result
}

// AesEncryptCtrMode encrypts plaintext using AES-CTR.
// The ciphertext is prefixed with the random IV so the caller does not need to manage it separately.
func AesEncryptCtrMode(plainText, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, block.BlockSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("generate iv: %w", err)
	}

	stream := cipher.NewCTR(block, iv)
	cipherText := make([]byte, len(plainText))
	stream.XORKeyStream(cipherText, plainText)

	return append(iv, cipherText...), nil
}

// AesDecryptCtrMode decrypts ciphertext produced by AesEncryptCtrMode.
// The first blockSize bytes are treated as the IV.
func AesDecryptCtrMode(encryptData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	if len(encryptData) < blockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := encryptData[:blockSize]
	cipherText := encryptData[blockSize:]

	stream := cipher.NewCTR(block, iv)
	plainText := make([]byte, len(cipherText))
	stream.XORKeyStream(plainText, cipherText)
	return plainText, nil
}
