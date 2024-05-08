package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
)

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
	logger.Debug("Received JSON message from " + c.Request.Header["X-Forwarded-For"][0])
	authorized, subscription := checkAuthorization(c)
	if authorized {
		var msg Message
		if err := c.BindJSON(&msg); err != nil {
			logger.Error("Invalid JSON from "+c.Request.Header["X-Forwarded-For"][0], zap.Error(err))
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
				sendWithFormat(subscription.ChatID, decrypted, msg.Format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				logger.Info("Received message: " + msg.Msg)
				// bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg.Msg))
				sendWithFormat(subscription.ChatID, msg.Msg, msg.Format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		}
	} else {
		logger.Error("Invalid UUID or not subscribed from "+c.Request.Header["X-Forwarded-For"][0], zap.Error(fmt.Errorf("invalid UUID or not subscribed")))
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleGet(c *gin.Context) {
	logger.Debug("Received GET message from " + c.Request.Header["X-Forwarded-For"][0])
	authorized, subscription := checkAuthorization(c)
	if authorized {
		msg := c.Query("msg")
		encrypted := c.Query("encrypted")
		format := c.Query("format")
		if format == "" {
			format = "markdown"
		}
		if msg != "" {
			if encrypted == "true" {
				decrypted, err := decrypt(msg, subscription.AESKey)
				if err != nil {
					logger.Error("Failed to decrypt message from: "+c.Request.Header["X-Forwarded-For"][0], zap.Error(err))
					c.JSON(http.StatusBadRequest, gin.H{
						"message": "Failed to decrypt message",
					})
					return
				}
				// bot.Send(tgbotapi.NewMessage(subscription.ChatID, decrypted))
				sendWithFormat(subscription.ChatID, decrypted, format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				// bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg))
				sendWithFormat(subscription.ChatID, msg, format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		} else {
			logger.Error("Invalid message from: "+c.Request.Header["X-Forwarded-For"][0], zap.Error(fmt.Errorf("invalid message")))
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid message",
			})
		}
	} else {
		logger.Error("Invalid UUID or not subscribed from: "+c.Request.Header["X-Forwarded-For"][0], zap.Error(fmt.Errorf("invalid UUID or not subscribed")))
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleForm(c *gin.Context) {
	logger.Debug("Received form message from " + c.Request.Header["X-Forwarded-For"][0])
	authorized, subscription := checkAuthorization(c)
	if authorized {
		msg := c.PostForm("msg")
		encrypted := c.PostForm("encrypted")
		format := c.PostForm("format")
		if format == "" {
			format = "markdown"
		}
		if msg != "" {
			if encrypted == "true" {
				decrypted, err := decrypt(msg, subscription.AESKey)
				if err != nil {
					logger.Error("Failed to decrypt message from: "+c.Request.Header["X-Forwarded-For"][0], zap.Error(err))
					c.JSON(http.StatusBadRequest, gin.H{
						"message": "Failed to decrypt message",
					})
					return
				}
				// bot.Send(tgbotapi.NewMessage(subscription.ChatID, decrypted))
				sendWithFormat(subscription.ChatID, decrypted, format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			} else {
				// bot.Send(tgbotapi.NewMessage(subscription.ChatID, msg))
				sendWithFormat(subscription.ChatID, msg, format)
				c.JSON(http.StatusOK, gin.H{
					"message": "Message sent",
				})
			}
		} else {
			logger.Error("Invalid message from "+c.Request.Header["X-Forwarded-For"][0], zap.Error(fmt.Errorf("invalid message")))
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid message",
			})
		}
	} else {
		logger.Error("Invalid UUID or not subscribed from "+c.Request.Header["X-Forwarded-For"][0], zap.Error(fmt.Errorf("invalid UUID or not subscribed")))
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Invalid UUID or not subscribed",
		})
	}
}

func handleFile(c *gin.Context) {
	logger.Debug("Received file from " + c.Request.Header["X-Forwarded-For"][0])
	authorized, subscription := checkAuthorization(c)
	if authorized {
		file, err := c.FormFile("file")
		file_caption := c.PostForm("caption")
		if err != nil {
			logger.Error("Failed to get file: "+file.Filename, zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid file",
			})
			return
		}
		if file != nil {
			sendFile(subscription.ChatID, file, file_caption)
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

func handleHTML(c *gin.Context) {
	uuidStr := c.Param("uuid")
	var article Article
	article_db.First(&article, "uuid = ?", uuidStr)
	if article.UUID != "" {
		htmlData, err := useTemplateRenderMarkdown([]byte(article.MarkdownText))
		if err != nil {
			logger.Error("Failed to render markdown with uuid: "+article.UUID, zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Failed to render markdown with uuid" + article.UUID,
			})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", htmlData)
	} else {
		logger.Error("Failed to get article with uuid: "+uuidStr, zap.Error(fmt.Errorf("failed to get article")))
		c.Redirect(http.StatusFound, "/404")
	}
}

func handleEmbedMarkdown(c *gin.Context, embed_path string) {
	indexHTML, err := useTemplateRenderEmbeddedFile(embed_path)
	if err != nil {
		logger.Error("Failed to render index.html", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to render index.html",
		})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}

func handleIndex(c *gin.Context) {
	handleEmbedMarkdown(c, "README.md")
}

func handleChangeLog(c *gin.Context) {
	handleEmbedMarkdown(c, "CHANGELOG.md")
}

func handleVersion(c *gin.Context) {
	versionData, err := loadEmbeddedFile("VERSION")
	if err != nil {
		logger.Error("Failed to load version file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to load version file",
		})
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", versionData)
}

func handle404(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"message": "404 Not Found",
	})
}
