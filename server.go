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
	ChatID      int64
	UserName    string
	NickName    string
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
	Format    string `json:"format"`
	Msg       string `json:"msg"`
}

type KeyboardCallbackData struct {
	Command          string `json:"command"`
	CommandChatID    int64  `json:"command_chat_id"`
	CurrentChatID    int64  `json:"current_chat_id"`
	CurrentMessageID int    `json:"current_message_id"`
}

var db *gorm.DB
var bot *tgbotapi.BotAPI
var config Config
var logger *zap.Logger

var config_path = flag.String("conf", "config.toml", "Path to config file")
var db_path = flag.String("db", "subscriptions.db", "Path to database file")

var versionStr = "v0.0.6"

var inMemoyrChatStore = make(map[int]int)

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
		{Command: "version", Description: "Get version"},
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

func sendHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	bot.Send(msg)
}

func sendText(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg)
}

func getChatInformation(chatID int64) (*tgbotapi.Chat, error) {

	chat, err := bot.GetChat(tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: chatID,
		}})
	if err != nil {
		logger.Fatal("Failed to get chat information", zap.Error(err))
		return nil, err
	}
	return &chat, nil
}

func handleSubscribe(chatID int64, managerID int64) {
	chat, err := getChatInformation(chatID)
	if err != nil {
		logger.Error("Failed to get chat information", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(managerID, "Failed to get chat information"))
		return
	}
	logger.Info("Received subscribe request", zap.Int64("chatID", chatID))
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = true
		subscription.UserName = chat.UserName
		subscription.NickName = chat.FirstName + " " + chat.LastName
		db.Save(&subscription)
		subscripedText := ""
		subscripedText += "You are already subscribed\n\n"
		subscripedText += "Your chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
		subscripedText += "Your username: `" + subscription.UserName + "`\n\n"
		subscripedText += "Your nickname: `" + subscription.NickName + "`\n\n"
		subscripedText += "Your UUID: `" + subscription.UUID + "`\n\n"
		subscripedText += "Your AES key: `" + subscription.AESKey + "`\n\n"
		sendMarkdownV2(managerID, subscripedText)
		return
	}
	uuidStr := uuid.New().String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1) // Remove dashes
	aesKey, err := generateRandomAESKey()
	if err != nil {
		logger.Error("Failed to generate AES key", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(managerID, "Failed to generate AES key"))
		return
	}
	userName := chat.UserName
	nickName := chat.FirstName + " " + chat.LastName
	db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true, AESKey: aesKey, UserName: userName, NickName: nickName})
	subscripedText := ""
	subscripedText += "Subscribed\n\n"
	subscripedText += "Your chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
	subscripedText += "Your username: `" + userName + "`\n\n"
	subscripedText += "Your nickname: `" + nickName + "`\n\n"
	subscripedText += "Your UUID: `" + uuidStr + "`\n\n"
	subscripedText += "Your AES key: `" + aesKey + "`\n\n"
	sendMarkdownV2(managerID, subscripedText)
}

func handleRegenerate(chatID int64, managerID int64) {
	chat, err := getChatInformation(chatID)
	if err != nil {
		logger.Error("Failed to get chat information", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(managerID, "Failed to get chat information"))
		return
	}
	uuidStr := uuid.New().String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1) // Remove dashes
	aesKey, err := generateRandomAESKey()
	if err != nil {
		logger.Error("Failed to generate AES key", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(managerID, "Failed to generate AES key"))
		return
	}
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.UUID = uuidStr
		subscription.AESKey = aesKey
		subscription.NickName = chat.FirstName + " " + chat.LastName
		subscription.UserName = chat.UserName
		db.Save(&subscription)
		subscriptionText := "Regenerated\n\n"
		subscriptionText += "Your UUID: `" + uuidStr + "`\n\n"
		subscriptionText += "Your AES key: `" + aesKey + "`\n\n"
		sendMarkdownV2(managerID, subscriptionText)
	} else {
		db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true, AESKey: aesKey, UserName: chat.UserName, NickName: chat.FirstName + " " + chat.LastName})
		subscriptionText := "Subscribed\n\n"
		subscriptionText += "Your UUID: `" + uuidStr + "`\n\n"
		subscriptionText += "Your AES key: `" + aesKey + "`\n\n"
		sendMarkdownV2(managerID, subscriptionText)
	}
}

func handleUnsubscribe(chatID int64, managerID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = false
		db.Save(&subscription)
		bot.Send(tgbotapi.NewMessage(managerID, "Unsubscribed"))
	} else {
		bot.Send(tgbotapi.NewMessage(managerID, "Invalid UUID or not subscribed"))
	}
}

func handleInfo(chatID int64, managerID int64) {
	chat, err := getChatInformation(chatID)
	if err != nil {
		logger.Error("Failed to get chat information", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(managerID, "Failed to get chat information"))
		return
	}
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.NickName = chat.FirstName + " " + chat.LastName
		subscription.UserName = chat.UserName
		db.Save(&subscription)
		msgText := ""
		msgText += "Your chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
		msgText += "Your username: `" + subscription.UserName + "`\n\n"
		msgText += "Your nickname: `" + subscription.NickName + "`\n\n"
		msgText += "Your UUID: `" + subscription.UUID + "`\n\n"
		msgText += "Your AES key: `" + subscription.AESKey + "`\n\n"
		if subscription.ReceiveMsgs {
			msgText += "You are subscribed to receive messages\n"
		} else {
			msgText += "You are not subscribed to receive messages\n"
		}
		sendMarkdownV2(managerID, msgText)
	} else {
		msgText := "Your Chat ID: `" + strconv.FormatInt(chatID, 10) + "`\n\n"
		msgText += "You are not subscribed to receive messages\n\n"
		msgText += "Use /subscribe to subscribe to receive messages"
		sendMarkdownV2(managerID, msgText)
	}
}

func handleHelp(chatID int64, managerID int64) {
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
	sendMarkdownV2(managerID, helpText)
}

func checkIsChannelAdmin(chatID int64, userID int64) bool {
	if chatID == userID {
		return true
	}
	admins, err := bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: chatID,
		},
	})
	if err != nil {
		logger.Error("Failed to get chat administrators", zap.Error(err))
		sendMarkdownV2(chatID, "Failed to get chat administrators")
		return false
	}
	for _, admin := range admins {
		if admin.User.ID == userID {
			return true
		}
	}
	return false
}

func checkBotIsChannelAdmin(chatID int64) bool {
	return checkIsChannelAdmin(chatID, bot.Self.ID)
}

func checkIsManager(chatID int64, userID int64) bool {
	return checkIsChannelAdmin(chatID, userID)
}

func checkIsForwardedChannelMessage(update tgbotapi.Update) bool {
	if update.Message.ForwardFromChat != nil && (update.Message.ForwardFromChat.Type == "channel" || update.Message.ForwardFromChat.Type == "supergroup" || update.Message.ForwardFromChat.Type == "group") {
		return true
	}
	return false
}

func getChatIDFromCommandArguments(args string) int64 {
	if args == "" {
		return 0
	}
	arguments := strings.TrimSpace(args)
	chatID, err := strconv.ParseInt(arguments, 10, 64)
	if err != nil {
		logger.Error("Failed to parse chat ID", zap.Error(err))
		return 0
	}
	return chatID
}

func processCommand(update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

	chatID := getChatIDFromCommandArguments(update.Message.CommandArguments())
	if chatID == 0 {
		chatID = update.Message.Chat.ID
	}

	logger.Info("Given Chat ID is ", zap.Int64("chat_id", chatID))

	if !checkIsManager(chatID, update.Message.From.ID) {
		logger.Info("Receive command but user is not manager")
		sendMarkdownV2(update.Message.Chat.ID, "Only the administrator of the channel/group can use this command")
		return
	}

	switch update.Message.Command() {
	case "start":
		handleHelp(chatID, update.Message.Chat.ID)
	case "subscribe":
		handleSubscribe(chatID, update.Message.Chat.ID)
	case "unsubscribe":
		handleUnsubscribe(chatID, update.Message.Chat.ID)
	case "regenerate":
		handleRegenerate(chatID, update.Message.Chat.ID)
	case "version":
		msg.Text = versionStr
	case "info":
		handleInfo(chatID, update.Message.Chat.ID)
	case "help":
		handleHelp(chatID, update.Message.Chat.ID)
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

func serilizeKeyboardCallbackData(data KeyboardCallbackData) string {
	return fmt.Sprintf("%s %s %s %s", data.Command, strconv.FormatInt(data.CommandChatID, 10), strconv.FormatInt(data.CurrentChatID, 10), strconv.Itoa(data.CurrentMessageID))
}

func deserializeKeyboardCallbackData(data string) KeyboardCallbackData {
	var keyboardCallbackData KeyboardCallbackData
	splitData := strings.Split(data, " ")
	if len(splitData) != 4 {
		return keyboardCallbackData
	}
	keyboardCallbackData.Command = splitData[0]
	keyboardCallbackData.CommandChatID, _ = strconv.ParseInt(splitData[1], 10, 64)
	keyboardCallbackData.CurrentChatID, _ = strconv.ParseInt(splitData[2], 10, 64)
	keyboardCallbackData.CurrentMessageID, _ = strconv.Atoi(splitData[3])
	return keyboardCallbackData
}

func processForwardedChannelMessage(update tgbotapi.Update) {
	logger.Info("Receive forwarded channel message", zap.Int64("user_id", update.Message.From.ID), zap.String("user_name", update.Message.Chat.UserName), zap.String("chat_name", update.Message.ForwardFromChat.Title))

	if !checkBotIsChannelAdmin(update.Message.ForwardFromChat.ID) {
		logger.Info("Receive forwarded channel message but bot is not channel admin")
		sendMarkdownV2(update.Message.Chat.ID, "Please add me as an administrator to the channel")
		return
	}

	if !checkIsChannelAdmin(update.Message.ForwardFromChat.ID, update.Message.From.ID) {
		logger.Info("Receive forwarded channel message but user is not channel admin")
		sendMarkdownV2(update.Message.Chat.ID, "Only the administrator of the channel can use this command")
		return
	}

	// send inline keyboard
	msgText := "Channel name: `" + update.Message.ForwardFromChat.Title + "`\n\n"
	msgText += "Channel id: `" + strconv.FormatInt(update.Message.ForwardFromChat.ID, 10) + "`\n\n"
	msgText += "Channel username: `" + update.Message.ForwardFromChat.UserName + "`\n\n"
	msgText += "Choose an option:"
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	logger.Info(serilizeKeyboardCallbackData(KeyboardCallbackData{Command: "subscribe", CommandChatID: update.Message.ForwardFromChat.ID, CurrentChatID: update.Message.Chat.ID, CurrentMessageID: update.Message.MessageID}))

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Subscribe", serilizeKeyboardCallbackData(KeyboardCallbackData{Command: "subscribe", CommandChatID: update.Message.ForwardFromChat.ID, CurrentChatID: update.Message.Chat.ID, CurrentMessageID: update.Message.MessageID}))),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Unsubscribe", serilizeKeyboardCallbackData(KeyboardCallbackData{Command: "unsubscribe", CommandChatID: update.Message.ForwardFromChat.ID, CurrentChatID: update.Message.Chat.ID, CurrentMessageID: update.Message.MessageID}))),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Regenerate", serilizeKeyboardCallbackData(KeyboardCallbackData{Command: "regenerate", CommandChatID: update.Message.ForwardFromChat.ID, CurrentChatID: update.Message.Chat.ID, CurrentMessageID: update.Message.MessageID}))),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Info", serilizeKeyboardCallbackData(KeyboardCallbackData{Command: "info", CommandChatID: update.Message.ForwardFromChat.ID, CurrentChatID: update.Message.Chat.ID, CurrentMessageID: update.Message.MessageID}))),
	)
	msg_, err := bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send inline keyboard", zap.Error(err))
		sendMarkdownV2(update.Message.Chat.ID, "Failed to send inline keyboard:"+err.Error())
	}
	logger.Info("Sent inline keyboard", zap.Int("message_id", msg_.MessageID))
	inMemoyrChatStore[update.Message.MessageID] = msg_.MessageID
}

func processCallbackQuery(update tgbotapi.Update) {
	logger.Info("Receive callback query", zap.String("data", update.CallbackQuery.Data))

	keyboardCallbackData := deserializeKeyboardCallbackData(update.CallbackQuery.Data)
	if keyboardCallbackData.Command == "" {
		logger.Error("Failed to deserialize keyboard callback data")
		bot.Send(tgbotapi.NewMessage(keyboardCallbackData.CurrentChatID, "Failed to deserialize keyboard callback data"))
	} else {
		switch keyboardCallbackData.Command {
		case "subscribe":
			handleSubscribe(keyboardCallbackData.CommandChatID, keyboardCallbackData.CurrentChatID)
		case "unsubscribe":
			handleUnsubscribe(keyboardCallbackData.CommandChatID, keyboardCallbackData.CurrentChatID)
		case "regenerate":
			handleRegenerate(keyboardCallbackData.CommandChatID, keyboardCallbackData.CurrentChatID)
		case "info":
			handleInfo(keyboardCallbackData.CommandChatID, keyboardCallbackData.CurrentChatID)
		default:
			logger.Error("Invalid command")
			bot.Send(tgbotapi.NewMessage(keyboardCallbackData.CurrentChatID, "Invalid command"))
		}
	}

	// delete inline keyboard

	messageID := inMemoyrChatStore[keyboardCallbackData.CurrentMessageID]

	_, err := bot.Request(tgbotapi.NewDeleteMessage(keyboardCallbackData.CurrentChatID, messageID))
	if err != nil {
		logger.Error("Failed to delete inline keyboard", zap.Error(err))
		bot.Send(tgbotapi.NewMessage(keyboardCallbackData.CurrentChatID, "Failed to delete inline keyboard"))
	}
	delete(inMemoyrChatStore, keyboardCallbackData.CurrentMessageID)
}

func processUpdate(update tgbotapi.Update) {
	if update.Message != nil {

		logger.Info("[%s] %s", zap.Int("update_id", update.UpdateID), zap.String("message", update.Message.Text))

		if checkIsForwardedChannelMessage(update) {
			processForwardedChannelMessage(update)
		}

		if update.Message.IsCommand() {
			processCommand(update)
			return
		}
	} else if update.CallbackQuery != nil {
		processCallbackQuery(update)
		return
	} else {
		return
	}
}

func startBot() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		processUpdate(update)
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
