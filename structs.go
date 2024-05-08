package main

import "html/template"

type Subscription struct {
	ChatID      int64
	UserName    string
	NickName    string
	UUID        string
	ReceiveMsgs bool
	AESKey      string `gorm:"size:32"`
}

type Article struct {
	UUID         string `json:"uuid"`
	MarkdownText string `json:"msg"`
}

type Config struct {
	TelegramToken  string `toml:"telegram_token"`
	TelegramAPIURL string `toml:"telegram_api_url"`
	GinAddress     string `toml:"gin_address"`
	PostURL        string `toml:"post_url"`
}

type Message struct {
	Encrypted bool   `json:"encrypted" default:"false" form:"encrypted"`
	Format    string `json:"format" default:"markdown" form:"format"`
	Msg       string `json:"msg" default:"Hello" form:"msg"`
}

type KeyboardCallbackData struct {
	Command          string `json:"command"`
	CommandChatID    int64  `json:"command_chat_id"`
	CurrentChatID    int64  `json:"current_chat_id"`
	CurrentMessageID int    `json:"current_message_id"`
}

type PageData struct {
	MarkdownContent template.HTML
}
