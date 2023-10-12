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
	Encrypted bool   `json:"encrypted"`
	Msg       string `json:"msg"`
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

func escapeMarkdownV2(text string) string {
	charactersToEscape := []string{"_", "[", "]", "(", ")", "~", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	for _, char := range charactersToEscape {
		text = strings.ReplaceAll(text, char, "\\"+char)
	}
	return text
}

func sendMarkdownV2(chatID int64, text string) {
	text = escapeMarkdownV2(text)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	bot.Send(msg)
}

func handleSubscribe(chatID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = true
		db.Save(&subscription)
		subscripedText := ""
		subscripedText += "You are already subscribed\n\n"
		subscripedText += "Your UUID: `" + subscription.UUID + "`\n\n"
		subscripedText += "Your AES key: `" + subscription.AESKey + "`\n\n"
		sendMarkdownV2(chatID, subscripedText)
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
	subscripedText := ""
	subscripedText += "Subscribed\n\n"
	subscripedText += "Your UUID: `" + uuidStr + "`\n\n"
	subscripedText += "Your AES key: `" + aesKey + "`\n\n"
	sendMarkdownV2(chatID, subscripedText)
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
		subscriptionText := "Regenerated\n\n"
		subscriptionText += "Your UUID: `" + uuidStr + "`\n\n"
		subscriptionText += "Your AES key: `" + aesKey + "`\n\n"
		sendMarkdownV2(chatID, subscriptionText)
	} else {
		db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true, AESKey: aesKey})
		subscriptionText := "Subscribed\n\n"
		subscriptionText += "Your UUID: `" + uuidStr + "`\n\n"
		subscriptionText += "Your AES key: `" + aesKey + "`\n\n"
		sendMarkdownV2(chatID, subscriptionText)
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
		msgText += "Your chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
		msgText += "Your UUID: `" + subscription.UUID + "`\n\n"
		msgText += "Your AES key: `" + subscription.AESKey + "`\n\n"
		if subscription.ReceiveMsgs {
			msgText += "You are subscribed to receive messages\n"
		} else {
			msgText += "You are not subscribed to receive messages\n"
		}
		sendMarkdownV2(chatID, msgText)
	} else {
		msgText := "Your Chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
		msgText += "You are not subscribed to receive messages\n\n"
		msgText += "Use /subscribe to subscribe to receive messages"
		sendMarkdownV2(chatID, msgText)
	}
}

func handleHelp(chatID int64) {

	subscription := Subscription{}
	db.First(&subscription, "chat_id = ?", chatID)
	uuidStr := ""
	if subscription.UUID != "" {
		uuidStr = subscription.UUID
	} else {
		uuidStr = "<UUID>"
	}

	helpText := `
Here are the available commands:

- /subscribe: Subscribe to receive messages
- /unsubscribe: Unsubscribe from receiving messages
- /regenerate: Regenerate UUID and AES key
- /info: Get your chat ID, UUID and AES key

After subscribing, you will receive a UUID and an AES key which can be used to send messages to your Telegram bot.

Here are the available endpoints and how to use them:

- **JSON Endpoint**:  
  POST to ` + "`" + config.PostURL + "/api/" + uuidStr + "/json`" + ` with JSON body {"encrypted": true, "msg": "<encrypted message>"} to send an encrypted message.
  
- **GET Endpoint**:  
  GET to ` + "`" + config.PostURL + "/api/" + uuidStr + "/get?msg=<message>&encrypted=<true/false>`" + ` to send a message.
  
- **Form Endpoint**:  
  POST to ` + "`" + config.PostURL + "/api/" + uuidStr + "/form`" + ` with form data msg=<message>, encrypted=<true/false> to send a message.
  
- **File Endpoint**:  
  POST to ` + "`" + config.PostURL + "/api/" + uuidStr + "/file`" + ` with form data file=<file> to send a file.

More information can be found at [nerdneilsfield/simple-telegram-notification-bot](https://github.com/nerdneilsfield/simple-telegram-notification-bot)
`
	sendMarkdownV2(chatID, helpText)
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
				msg.Text += "\n\n"
				msg.Text += "Use /unsubscribe to unsubscribe from receiving messages"
				msg.Text += "\n\n"
				msg.Text += "Use /regenerate to regenerate UUID and AES key"
				msg.Text += "\n\n"
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
			if msg.Encrypted {
				decrypted, err := decrypt(msg.Msg, subscription.AESKey)
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
				logger.Info("Received message: " + msg.Msg)
				bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg.Msg))
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

	flag.Parse()

	logger, _ = zap.NewProduction()
	defer logger.Sync()
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
