package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Subscription struct {
	ChatID      int64 `gorm:"primaryKey"`
	UUID        string
	ReceiveMsgs bool
	AESKey      string `gorm:"size:32"`
}

type Config struct {
	TelegramToken  string `toml:"telegram_token"`
	TelegramAPIURL string `toml:"telegram_api_url"`
	GinAddress     string `toml:"gin_address"`
	PostURL        string `toml:"post_url"`
}

type Message struct {
	encrypted bool   `json:"encrypted"`
	msg       string `json:"msg"`
}

var db *gorm.DB
var bot *tgbotapi.BotAPI
var config Config
var logger *zap.Logger

var config_path = flag.String("conf", "config.toml", "Path to config file")
var db_path = flag.String("db", "subscriptions.db", "Path to database file")

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open(*db_path), &gorm.Config{})
	if err != nil {
		logger.Fatal("Failed to connect database", zap.Error(err))
		panic("failed to connect database")
	}
	// Migrate the schema
	db.AutoMigrate(&Subscription{})
	logger.Info("Database connection initialized")
}

func initBot(token string, url string) {
	var err error
	bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, url)
	if err != nil {
		logger.Fatal("Failed to connect to Telegram API", zap.Error(err))
		panic(err)
	}
	logger.Info("Telegram bot connection initialized with username: " + bot.Self.UserName)
	// set init command
	commandConfig := tgbotapi.NewSetMyCommands([]tgbotapi.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "subscribe", Description: "Subscribe to receive messages"},
		{Command: "unsubscribe", Description: "Unsubscribe from receiving messages"},
		{Command: "regenerate", Description: "Regenerate UUID and AES key"},
		{Command: "info", Description: "Get your chat ID, UUID and AES key"},
		{Command: "help", Description: "Get help"},
	}...)
	bot.Request(commandConfig)
	logger.Info("Telegram bot commands set")
}

func handleSubscribe(chatID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = true
		db.Save(&subscription)
		bot.Send(tgbotapi.NewMessage(chatID, "Turn on notifications wigth UUID: "+subscription.UUID))
		bot.Send(tgbotapi.NewMessage(chatID, "Your AES key: "+subscription.AESKey))
		bot.Send(tgbotapi.NewMessage(chatID, "Your uuid string is: "+subscription.UUID))
		return
	}
	uuidStr := uuid.New().String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1) // Remove dashes
	aesKey, err := generateRandomAESKey()
	if err != nil {
		logger.Error("Failed to generate AES key", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(chatID, "Failed to generate AES key"))
		return
	}
	db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true, AESKey: aesKey})
	bot.Send(tgbotapi.NewMessage(chatID, "Subscribed with UUID: "+uuidStr))
	bot.Send(tgbotapi.NewMessage(chatID, "Your AES key: "+aesKey))
	bot.Send(tgbotapi.NewMessage(chatID, "Your uuid string is: "+uuidStr))
}

func handleRegenerate(chatID int64) {
	uuidStr := uuid.New().String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1) // Remove dashes
	aesKey, err := generateRandomAESKey()
	if err != nil {
		logger.Error("Failed to generate AES key", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(chatID, "Failed to generate AES key"))
		return
	}
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.UUID = uuidStr
		subscription.AESKey = aesKey
		db.Save(&subscription)
		bot.Send(tgbotapi.NewMessage(chatID, "Regenerated UUID: "+uuidStr))
		bot.Send(tgbotapi.NewMessage(chatID, "Your new AES key: "+aesKey))
		bot.Send(tgbotapi.NewMessage(chatID, "Your uuid string is: "+uuidStr))
	} else {
		db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true, AESKey: aesKey})
		bot.Send(tgbotapi.NewMessage(chatID, "Subscribed with UUID: "+uuidStr))
		bot.Send(tgbotapi.NewMessage(chatID, "Your AES key: "+aesKey))
		bot.Send(tgbotapi.NewMessage(chatID, "Your uuid string is: "+uuidStr))
	}
}

func handleUnsubscribe(chatID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = false
		db.Save(&subscription)
		bot.Send(tgbotapi.NewMessage(chatID, "Unsubscribed"))
	} else {
		bot.Send(tgbotapi.NewMessage(chatID, "Invalid UUID or not subscribed"))
	}
}

func handleInfo(chatID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		msgText := ""
		msgText += "Your chat ID: " + strconv.FormatInt(chatID, 10) + "\n"
		msgText += "Your UUID: " + subscription.UUID + "\n"
		msgText += "Your AES key: " + subscription.AESKey + "\n"
		if subscription.ReceiveMsgs {
			msgText += "You are subscribed to receive messages\n"
		} else {
			msgText += "You are not subscribed to receive messages\n"
		}
		bot.Send(tgbotapi.NewMessage(chatID, msgText))
	} else {
		bot.Send(tgbotapi.NewMessage(chatID, "Your chat ID: "+strconv.FormatInt(chatID, 10)))
		bot.Send(tgbotapi.NewMessage(chatID, "Invalid UUID or not subscribed"))
	}
}

func handleHelp(chatID int64) {
	helpText := ""
	helpText += "Use /subscribe to subscribe to receive messages\n"
	helpText += "Use /unsubscribe to unsubscribe from receiving messages\n"
	helpText += "Use /regenerate to regenerate UUID and AES key\n"
	helpText += "Use /info to get your chat ID, UUID and AES key\n"
	helpText += "After subscribed, you will get a UUID and an AES key\n"
	helpText += "You can use the UUID and AES key to send messages to your Telegram bot\n"
	helpText += "POST to " + config.PostURL + "/api/<UUID>/json with JSON body {\"encrypted\": true, \"msg\": \"<encrypted message>\"} to send an encrypted message\n"
	helpText += "GET to " + config.PostURL + "/api/<UUID>/get?msg=<message>&encrypted=<true/false> to send a message\n"
	helpText += "POST to " + config.PostURL + "/api/<UUID>/form with form data msg=<message>, encryped=<true/false> to send a message\n"
	helpText += "POST to " + config.PostURL + "/api/<UUID>/file with form data file=<file> to send a file\n"
	bot.Send(tgbotapi.NewMessage(chatID, helpText))
}

func startBot() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	// if err != nil {
	// 	panic(err)
	// }

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			switch update.Message.Command() {
			case "start":
				handleHelp(update.Message.Chat.ID)
			case "subscribe":
				handleSubscribe(update.Message.Chat.ID)
			case "unsubscribe":
				handleUnsubscribe(update.Message.Chat.ID)
			case "regenerate":
				handleRegenerate(update.Message.Chat.ID)
			case "info":
				handleInfo(update.Message.Chat.ID)
			case "help":
				handleHelp(update.Message.Chat.ID)
			default:
				msg.Text = "I don't know that command"
				msg.Text += "\n\n"
				msg.Text += "Use /subscribe to subscribe to receive messages"
				msg.Text += "\n"
				msg.Text += "Use /unsubscribe to unsubscribe from receiving messages"
				msg.Text += "\n"
				msg.Text += "Use /regenerate to regenerate UUID and AES key"
			}
			if msg.Text != "" {
				bot.Send(msg)
			}
		}
	}
}

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

func checkAuthorization(c *gin.Context) (bool, *Subscription) {
	uuidStr := c.Param("uuid")
	var subscription Subscription
	db.First(&subscription, "uuid = ?", uuidStr)
	if subscription.UUID != "" {
		if subscription.ReceiveMsgs {
			return true, &subscription
		} else {
			return false, nil
		}
	} else {
		return false, nil
	}
}

func handleJSON(c *gin.Context) {
	authorized, subscription := checkAuthorization(c)
	if authorized {
		var msg Message
		if err := c.BindJSON(&msg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid JSON",
			})
			return
		} else {
			if msg.encrypted {
				decrypted, err := decrypt(msg.msg, subscription.AESKey)
				if err != nil {
					logger.Error("Failed to decrypt message", zap.Error(err))
					c.JSON(http.StatusBadRequest, gin.H{
						"message": "Failed to decrypt message",
					})
					return
				}
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, decrypted))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				logger.Info("Received message: " + msg.msg)
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg.msg))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleGet(c *gin.Context) {
	authorized, subscription := checkAuthorization(c)
	if authorized {
		msg := c.Query("msg")
		encrypted := c.Query("encrypted")
		if msg != "" {
			if encrypted == "true" {
				decrypted, err := decrypt(msg, subscription.AESKey)
				if err != nil {
					logger.Error("Failed to decrypt message", zap.Error(err))
					c.JSON(http.StatusBadRequest, gin.H{
						"message": "Failed to decrypt message",
					})
					return
				}
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, decrypted))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid message",
			})
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleForm(c *gin.Context) {
	authorized, subscription := checkAuthorization(c)
	if authorized {
		msg := c.PostForm("msg")
		encrypted := c.PostForm("encrypted")
		if msg != "" {
			if encrypted == "true" {
				decrypted, err := decrypt(msg, subscription.AESKey)
				if err != nil {
					logger.Error("Failed to decrypt message", zap.Error(err))
					c.JSON(http.StatusBadRequest, gin.H{
						"message": "Failed to decrypt message",
					})
					return
				}
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, decrypted))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg))
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid message",
			})
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleFile(c *gin.Context) {
	authorized, subscription := checkAuthorization(c)
	if authorized {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid file",
			})
			return
		}
		if file != nil {
			fileBytes, err := file.Open()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "Invalid file",
				})
				return
			}
			logger.Info("Received file: " + file.Filename + " with size: " + strconv.FormatInt(file.Size, 10))
			doc := tgbotapi.FileBytes{Name: file.Filename}
			content, err := io.ReadAll(fileBytes)
			if err != nil {
				logger.Error("Failed to read file", zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "Failed to read file",
				})
				return
			}
			doc.Bytes = content
			bot.Send(tgbotapi.NewDocument(subscription.ChatID, doc))
			c.JSON(http.StatusOK, gin.H{
				"message": "File sent",
			})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid file",
			})
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func main() {

	logger, _ = zap.NewDevelopment()
	// Parse config file
	if _, err := toml.DecodeFile(*config_path, &config); err != nil {
		logger.Fatal("Failed to parse config file:", zap.Error(err))
		panic(err)
	}

	logger.Info("Telegram API URL: " + config.TelegramAPIURL)
	logger.Info("Post URL: " + config.PostURL)
	logger.Info("Gin Address: " + config.GinAddress)

	// Initialize the database and bot
	initDB()
	initBot(config.TelegramToken, config.TelegramAPIURL)

	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		log.Printf("Received request from %s", c.ClientIP())
		c.JSON(http.StatusOK, gin.H{
			"message": "Hello World!",
		})
	})

	router.POST("/api/:uuid/json", handleJSON)
	router.GET("/api/:uuid/get", handleGet)
	router.POST("/api/:uuid/form", handleForm)
	router.POST("/api/:uuid/file", handleFile)

	go startBot()

	router.Run(config.GinAddress) // listen and serve on configured address
}
