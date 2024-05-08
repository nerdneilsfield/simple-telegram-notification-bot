package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"
)

func generateRandomAESKey() (string, error) {
	// 为 AES-256，密钥长度为 32 字节
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func decrypt(encrypted string, key string) (string, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		logger.Error("Failed to decode key", zap.Error(err))
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		logger.Error("Failed to decode ciphertext", zap.Error(err))
		return "", err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		logger.Error("Failed to create cipher", zap.Error(err))
		return "", err
	}

	if len(ciphertext) < aes.BlockSize {
		logger.Error("Ciphertext too short")
		return "", fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	return string(ciphertext), nil
}
