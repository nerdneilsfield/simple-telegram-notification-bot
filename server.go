package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Subscription struct {
	UUID        string `gorm:"primaryKey"`
	ChatID      int64
	ReceiveMsgs bool
}

type Config struct {
	TelegramToken  string `toml:"telegram_token"`
	TelegramAPIURL string `toml:"telegram_api_url"`
	GinAddress     string `toml:"gin_address"`
	PostURL        string `toml:"post_url"`
}

type Message struct {
	msg string `json:"msg"`
}

var db *gorm.DB
var bot *tgbotapi.BotAPI
var config Config

var config_path = flag.String("conf", "config.toml", "Path to config file")
var db_path = flag.String("db", "subscriptions.db", "Path to database file")

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open(*db_path), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Migrate the schema
	db.AutoMigrate(&Subscription{})
	log.Printf("Database initialized")
}

func initBot(token string, url string) {
	var err error
	bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, url)
	if err != nil {
		panic(err)
	}
	// set init command
	commandConfig := tgbotapi.NewSetMyCommands([]tgbotapi.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "subscribe", Description: "Subscribe to receive messages"},
		{Command: "unsubscribe", Description: "Unsubscribe from receiving messages"},
	}...)
	bot.Request(commandConfig)
	log.Printf("Authorized on account %s", bot.Self.UserName)
}

func handleSubscribe(chatID int64) {
	var subscription Subscription
	db.First(&subscription, "chat_id = ?", chatID)
	if subscription.UUID != "" {
		subscription.ReceiveMsgs = true
		db.Save(&subscription)
		bot.Send(tgbotapi.NewMessage(chatID, "Turn on notifications wigth UUID: "+subscription.UUID))
		bot.Send(tgbotapi.NewMessage(chatID, "Post to "+config.PostURL+"/api/"+subscription.UUID+" to send a message"))
		return
	}
	uuidStr := uuid.New().String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1) // Remove dashes
	db.Create(&Subscription{UUID: uuidStr, ChatID: chatID, ReceiveMsgs: true})
	bot.Send(tgbotapi.NewMessage(chatID, "Subscribed with UUID: "+uuidStr))
	bot.Send(tgbotapi.NewMessage(chatID, "Post to "+config.PostURL+"/api/"+uuidStr+" to send a message"))
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
				msg.Text = "Hello, welcome to the notification bot"
				msg.Text += "\n\n"
				msg.Text += "Use /subscribe to subscribe to receive messages"
				msg.Text += "\n"
				msg.Text += "Use /unsubscribe to unsubscribe from receiving messages"
			case "subscribe":
				handleSubscribe(update.Message.Chat.ID)
			case "unsubscribe":
				handleUnsubscribe(update.Message.Chat.ID)
			default:
				msg.Text = "I don't know that command"
				msg.Text += "\n\n"
				msg.Text += "Use /subscribe to subscribe to receive messages"
				msg.Text += "\n"
				msg.Text += "Use /unsubscribe to unsubscribe from receiving messages"
			}
			bot.Send(msg)
		}
	}
}

func main() {

	// Parse config file

	if _, err := toml.DecodeFile(*config_path, &config); err != nil {
		panic(err)
	}

	log.Printf("Telegram token: %s", config.TelegramToken)
	log.Printf("Telegram API URL: %s", config.TelegramAPIURL)
	log.Printf("Gin address: %s", config.GinAddress)

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

	router.POST("/api/:uuid", func(c *gin.Context) {
		uuidStr := c.Param("uuid")
		log.Printf("Received request from %s with UUID %s", c.ClientIP(), uuidStr)
		var subscription Subscription
		db.First(&subscription, "uuid = ? AND receive_msgs = ?", uuidStr, true)
		if subscription.UUID != "" {
			var msg Message
			if err := c.BindJSON(&msg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "Invalid JSON",
				})
				return
			}
			bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg.msg))
			c.JSON(http.StatusOK, gin.H{
				"message": "Message sent",
			})
		} else {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "Invalid UUID or not subscribed",
			})
		}
	})

	go startBot()

	router.Run(config.GinAddress) // listen and serve on configured address
}
