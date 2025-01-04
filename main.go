package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "modernc.org/sqlite"
)

type BotManager struct {
	bots    map[string]*tgbotapi.BotAPI
	creator map[string]int64
	mu      sync.RWMutex
	db      *sql.DB
}

func NewBotManager(db *sql.DB) *BotManager {
	return &BotManager{
		bots:    make(map[string]*tgbotapi.BotAPI),
		creator: make(map[string]int64),
		db:      db,
	}
}

func (m *BotManager) AddBot(token string, creatorID int64) error {
	log.Printf("Attempting to add bot with token: %s, creator ID: %d", token, creatorID)
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Printf("Failed to create bot API for token %s: %v", token, err)
		return err
	}
	log.Printf("Bot API created successfully for token: %s", token)

	m.mu.Lock()
	m.bots[token] = bot
	m.creator[token] = creatorID
	m.mu.Unlock()
	log.Printf("Bot %s added to the manager's in-memory storage.", token)

	go m.startBot(bot, creatorID)
	log.Printf("Bot %s started.", token)

	// 检查bot是否已存在
	var exists bool
	err = m.db.QueryRow("SELECT EXISTS(SELECT 1 FROM bots WHERE token = ?)", token).Scan(&exists)
	if err != nil {
		log.Printf("Failed to check bot existence for token %s: %v", token, err)
		return err
	}

	if exists {
		log.Printf("Bot with token %s already exists in the database.", token)
		return nil // 早期返回，无需重新插入
	}

	// 插入新bot到数据库中
	_, err = m.db.Exec("INSERT INTO bots (token, creator_id) VALUES (?, ?)", token, creatorID)
	if err != nil {
		log.Printf("Failed to insert bot with token %s into database: %v", token, err)
		return err
	}
	log.Printf("Bot with token %s added to the database successfully.", token)

	return nil
}

// 获取用户的申诉次数
func (m *BotManager) getAppealCount(token string, userID int64) int {
	var appealCountsStr string
	err := m.db.QueryRow("SELECT appeal_counts FROM bots WHERE token = ?", token).Scan(&appealCountsStr)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get appeal counts for bot %s: %v", token, err)
		return 0
	}

	if appealCountsStr == "" {
		return 0
	}

	var appealCounts map[string]int
	if err := json.Unmarshal([]byte(appealCountsStr), &appealCounts); err != nil {
		log.Printf("Failed to unmarshal appeal counts: %v", err)
		return 0
	}

	return appealCounts[strconv.FormatInt(userID, 10)]
}

// 增加用户的申诉次数
func (m *BotManager) incrementAppealCount(token string, userID int64) error {
	var appealCountsStr string
	err := m.db.QueryRow("SELECT appeal_counts FROM bots WHERE token = ?", token).Scan(&appealCountsStr)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get appeal counts for bot %s: %v", token, err)
		return err
	}

	appealCounts := make(map[string]int)
	if appealCountsStr != "" {
		if err := json.Unmarshal([]byte(appealCountsStr), &appealCounts); err != nil {
			log.Printf("Failed to unmarshal appeal counts: %v", err)
			return err
		}
	}

	userIDStr := strconv.FormatInt(userID, 10)
	appealCounts[userIDStr]++

	updatedAppealCounts, err := json.Marshal(appealCounts)
	if err != nil {
		log.Printf("Failed to marshal updated appeal counts: %v", err)
		return err
	}

	_, err = m.db.Exec("UPDATE bots SET appeal_counts = ? WHERE token = ?", string(updatedAppealCounts), token)
	if err != nil {
		log.Printf("Failed to update appeal counts for bot %s: %v", token, err)
		return err
	}

	return nil
}

func (m *BotManager) isUserBlocked(token string, userID int64) bool {
	var blockedUsers string
	err := m.db.QueryRow("SELECT blocked_users FROM bots WHERE token = ?", token).Scan(&blockedUsers)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get blocked users for bot %s: %v", token, err)
		return false
	}

	if blockedUsers == "" {
		return false // No blocked users for this bot
	}

	blockedList := strings.Split(blockedUsers, ",")
	for _, idStr := range blockedList {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil && id == userID {
			return true // User is blocked
		}
	}
	return false // User is not blocked
}

// 在 BotManager 结构体中添加一个方法，用于添加用户到黑名单
func (m *BotManager) blockUser(token string, userID int64) error {
	var blockedUsers string
	err := m.db.QueryRow("SELECT blocked_users FROM bots WHERE token = ?", token).Scan(&blockedUsers)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get blocked users for bot %s: %v", token, err)
		return err
	}

	blockedList := strings.Split(blockedUsers, ",")
	for _, idStr := range blockedList {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil && id == userID {
			log.Printf("User ID: %d is already in the block list for bot %s.", userID, token)
			return nil // User already blocked
		}
	}

	if blockedUsers != "" {
		blockedList = append(blockedList, strconv.FormatInt(userID, 10))
	} else {
		blockedList = []string{strconv.FormatInt(userID, 10)}
	}

	newBlockedUsers := strings.Join(blockedList, ",")

	_, err = m.db.Exec("UPDATE bots SET blocked_users = ? WHERE token = ?", newBlockedUsers, token)
	if err != nil {
		log.Printf("Failed to add user to block list for bot %s: %v", token, err)
		return err
	}
	log.Printf("User ID: %d added to the block list for bot %s.", userID, token)
	return nil
}

func (m *BotManager) unblockUser(token string, userID int64) error {
	var blockedUsers string
	err := m.db.QueryRow("SELECT blocked_users FROM bots WHERE token = ?", token).Scan(&blockedUsers)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get blocked users for bot %s: %v", token, err)
		return err
	}

	if blockedUsers == "" {
		log.Printf("No blocked users found for bot %s", token)
		return nil
	}

	blockedList := strings.Split(blockedUsers, ",")
	newBlockedList := []string{}
	for _, idStr := range blockedList {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		if id != userID {
			newBlockedList = append(newBlockedList, idStr)
		}
	}

	newBlockedUsers := strings.Join(newBlockedList, ",")

	_, err = m.db.Exec("UPDATE bots SET blocked_users = ? WHERE token = ?", newBlockedUsers, token)
	if err != nil {
		log.Printf("Failed to remove user from block list for bot %s: %v", token, err)
		return err
	}

	// Reset the appeal count when unbanning user
	var appealCountsStr string
	err = m.db.QueryRow("SELECT appeal_counts FROM bots WHERE token = ?", token).Scan(&appealCountsStr)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get appeal counts for bot %s: %v", token, err)
		return err
	}

	appealCounts := make(map[string]int)
	if appealCountsStr != "" {
		if err := json.Unmarshal([]byte(appealCountsStr), &appealCounts); err != nil {
			log.Printf("Failed to unmarshal appeal counts: %v", err)
		}
	}

	userIDStr := strconv.FormatInt(userID, 10)
	delete(appealCounts, userIDStr) // Remove the user from the map

	updatedAppealCounts, err := json.Marshal(appealCounts)
	if err != nil {
		log.Printf("Failed to marshal updated appeal counts: %v", err)
		return err
	}
	_, err = m.db.Exec("UPDATE bots SET appeal_counts = ? WHERE token = ?", string(updatedAppealCounts), token)
	if err != nil {
		log.Printf("Failed to update appeal counts for bot %s: %v", token, err)
		return err
	}
	log.Printf("User ID: %d removed from the block list and appeal count reset for bot %s.", userID, token)

	return nil
}

func (m *BotManager) handleBotCommands(bot *tgbotapi.BotAPI, update *tgbotapi.Update, creatorID int64) {
	botToken := bot.Token // Get the bot token here
	if update.Message.Command() == "start" {
		// Handle /start command
		userID := update.Message.From.ID
		userName := update.Message.From.UserName
		if userName == "" {
			userName = update.Message.From.FirstName + " " + update.Message.From.LastName
		}
		if userName == " " {
			userName = update.Message.From.FirstName
		}

		startMessage := tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户 %s (ID: %d) 发起了 /start 命令。\n\n选择操作:", userName, userID))

		// 创建封禁按钮
		banButton := tgbotapi.NewInlineKeyboardButtonData("封禁", fmt.Sprintf("ban_%d", userID))

		// 创建解禁按钮
		unbanButton := tgbotapi.NewInlineKeyboardButtonData("解禁", fmt.Sprintf("unban_%d", userID))

		// 将按钮添加到键盘中
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(banButton, unbanButton),
		)
		startMessage.ReplyMarkup = keyboard

		if _, err := bot.Send(startMessage); err != nil {
			log.Printf("Failed to send /start message to creator: %v", err)
		} else {
			log.Printf("Sent /start message to creator for user ID: %d", userID)
		}
		return // Skip forwarding for /start command
	}

	switch update.Message.Command() {
	case "getbans":
		// Handle /getbans command
		var blockedUsers string
		err := m.db.QueryRow("SELECT blocked_users FROM bots WHERE token = ?", botToken).Scan(&blockedUsers)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Failed to get blocked users for bot %s: %v", botToken, err)
			bot.Send(tgbotapi.NewMessage(creatorID, "Failed to get blocked users."))
			return
		}

		if blockedUsers == "" {
			bot.Send(tgbotapi.NewMessage(creatorID, "当前没有封禁用户"))
			return
		}
		bot.Send(tgbotapi.NewMessage(creatorID, fmt.Sprintf("封禁列表: %s", blockedUsers)))
		return

	case "ban":
		// Handle /ban command
		args := update.Message.CommandArguments()
		if args == "" {
			bot.Send(tgbotapi.NewMessage(creatorID, "请提供要封禁的 Telegram ID，例如：/ban 123456"))
			return
		}
		userID, err := strconv.ParseInt(args, 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(creatorID, "无效的 Telegram ID"))
			return
		}
		if err := m.blockUser(botToken, userID); err != nil {
			log.Printf("Failed to block user using /ban command: %v", err)
			bot.Send(tgbotapi.NewMessage(creatorID, "Failed to block user"))
			return
		}
		bot.Send(tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户ID: %d 已被封禁", userID)))
		return
	case "unban":
		// Handle /unban command
		args := update.Message.CommandArguments()
		if args == "" {
			bot.Send(tgbotapi.NewMessage(creatorID, "请提供要解封的 Telegram ID，例如：/unban 123456"))
			return
		}
		userID, err := strconv.ParseInt(args, 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(creatorID, "无效的 Telegram ID"))
			return
		}
		if err := m.unblockUser(botToken, userID); err != nil {
			log.Printf("Failed to unblock user using /unban command: %v", err)
			bot.Send(tgbotapi.NewMessage(creatorID, "Failed to unblock user"))
			return
		}
		bot.Send(tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户ID: %d 已被解封", userID)))
		return
	}
}

func (m *BotManager) startBot(bot *tgbotapi.BotAPI, creatorID int64) {
	botToken := bot.Token // Get the bot token here
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	log.Printf("Starting bot with creator ID: %d", creatorID)
	updates := bot.GetUpdatesChan(u)

	appeals := make(map[int64]bool)
	for update := range updates {
		if update.Message != nil {
			log.Printf("Received a message from user ID: %d in chat ID: %d, text: %s", update.Message.From.ID, update.Message.Chat.ID, update.Message.Text)

			userID := update.Message.From.ID
			if appeals[userID] {
				appealText := update.Message.Text
				appealForward := tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户 %d 发起申诉: %s", userID, appealText))
				if _, err := bot.Send(appealForward); err != nil {
					log.Printf("Failed to send appeal message to creator: %v", err)
				}
				log.Printf("Received appeal message from user ID: %d, forwarding to creator.", userID)
				delete(appeals, userID) // Clear the flag

				// 增加申诉次数
				if err := m.incrementAppealCount(botToken, userID); err != nil {
					log.Printf("Failed to increment appeal count for user %d of bot %s : %v", userID, botToken, err)
				}

				// 获取申诉次数
				appealCount := m.getAppealCount(botToken, userID)
				if appealCount >= 3 {
					if err := m.blockUser(botToken, userID); err != nil {
						log.Printf("Failed to block user using /ban command: %v", err)
					}
					noAppealMsg := tgbotapi.NewMessage(userID, "你的申诉次数已达上限，已被永久封禁。")
					if _, err := bot.Send(noAppealMsg); err != nil {
						log.Printf("Failed to send no appeal message to user %d of bot %s : %v", userID, botToken, err)
					}
				}

				continue
			}

			if update.Message.IsCommand() && update.Message.From.ID == creatorID {
				m.handleBotCommands(bot, &update, creatorID)
				continue
			} else if update.Message.IsCommand() {
				m.handleBotCommands(bot, &update, creatorID)
				continue
			}

			if update.Message.From.ID == creatorID {
				m.handleReplyMessage(bot, update.Message)
			} else {
				m.handleIncomingMessage(bot, update.Message, creatorID, bot, botToken)
			}
		} else if update.CallbackQuery != nil {
			// Handle button clicks
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
			if _, err := bot.Request(callback); err != nil {
				log.Printf("Error processing callback: %v", err)
				continue
			}

			callbackData := update.CallbackQuery.Data
			log.Printf("Received a callback query with data: %s", callbackData)

			if strings.HasPrefix(callbackData, "appeal_") {
				userIDStr := strings.TrimPrefix(callbackData, "appeal_")
				userID, err := strconv.ParseInt(userIDStr, 10, 64)
				if err != nil {
					log.Printf("Invalid userID in callback: %v", err)
					continue
				}

				if m.getAppealCount(botToken, userID) >= 3 {
					noAppealMsg := tgbotapi.NewMessage(userID, "你的申诉次数已达上限，已被永久封禁。")
					if _, err := bot.Send(noAppealMsg); err != nil {
						log.Printf("Failed to send no appeal message to user %d of bot %s : %v", userID, botToken, err)
					}
					continue
				}

				// Send a message asking for appeal information
				appealMsg := tgbotapi.NewMessage(userID, "请在此输入你的申诉信息：")
				if _, err := bot.Send(appealMsg); err != nil {
					log.Printf("Failed to send appeal message to user: %v", err)
					continue
				}
				appeals[userID] = true

				continue
			}

			if strings.HasPrefix(callbackData, "ban_") {
				userIDStr := strings.TrimPrefix(callbackData, "ban_")
				userID, err := strconv.ParseInt(userIDStr, 10, 64)
				if err != nil {
					log.Printf("Invalid userID in callback: %v", err)
					continue
				}
				log.Printf("Creator requested to ban user ID: %d for bot %s", userID, botToken)

				// 将用户添加到黑名单
				if err := m.blockUser(botToken, userID); err != nil {
					log.Printf("Failed to block user: %v", err)
					continue
				}

				banMsg := tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户ID: %d 已被封禁", userID))
				if _, err := bot.Send(banMsg); err != nil {
					log.Printf("Failed to send ban confirmation message to creator: %v", err)
				}
			} else if strings.HasPrefix(callbackData, "unban_") {
				userIDStr := strings.TrimPrefix(callbackData, "unban_")
				userID, err := strconv.ParseInt(userIDStr, 10, 64)
				if err != nil {
					log.Printf("Invalid userID in callback: %v", err)
					continue
				}
				log.Printf("Creator requested to unban user ID: %d for bot %s", userID, botToken)
				// 将用户从黑名单删除
				if err := m.unblockUser(botToken, userID); err != nil {
					log.Printf("Failed to unblock user: %v", err)
					continue
				}
				unbanMsg := tgbotapi.NewMessage(creatorID, fmt.Sprintf("用户ID: %d 已被解禁", userID))
				if _, err := bot.Send(unbanMsg); err != nil {
					log.Printf("Failed to send unban confirmation message to creator: %v", err)
				}
			}
		}
	}
}

func (m *BotManager) handleIncomingMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, creatorID int64, botAPI *tgbotapi.BotAPI, botToken string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userID := message.From.ID

	if m.isUserBlocked(botToken, userID) {
		log.Printf("User ID: %d is blocked for bot %s, not forwarding message.", userID, botToken)

		if m.getAppealCount(botToken, userID) >= 3 {
			blockedMsg := tgbotapi.NewMessage(userID, "你已被永久封禁，无法发送消息。")
			if _, err := botAPI.Send(blockedMsg); err != nil {
				log.Printf("Failed to send blocked message to user: %v", err)
			}
			return
		}

		// Create inline keyboard for appeal
		appealButton := tgbotapi.NewInlineKeyboardButtonData("误伤了？申诉一下", fmt.Sprintf("appeal_%d", userID))
		keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(appealButton))

		blockedMsg := tgbotapi.NewMessage(userID, "你已被封禁，无法发送消息。")
		blockedMsg.ReplyMarkup = keyboard

		if _, err := botAPI.Send(blockedMsg); err != nil {
			log.Printf("Failed to send blocked message with appeal button to user: %v", err)
		}
		return
	}

	log.Printf("Forwarding message from user ID: %d to creator ID: %d", message.From.ID, creatorID)
	// Forward message to creator
	msg := tgbotapi.NewForward(creatorID, message.Chat.ID, message.MessageID)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error forwarding message: %v", err)
	} else {
		log.Println("Message forwarded successfully.")
	}
}

func (m *BotManager) handleReplyMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Confirm ReplyToMessage and its properties are available
	if message.ReplyToMessage != nil && message.ReplyToMessage.ForwardFrom != nil {
		originalSenderID := message.ReplyToMessage.ForwardFrom.ID
		log.Printf("Attempting to reply to user ID: %d", originalSenderID)

		// Send reply
		replyMsg := tgbotapi.NewMessage(originalSenderID, message.Text)
		if _, err := bot.Send(replyMsg); err != nil {
			log.Printf("Error sending reply message: %v", err)
		} else {
			log.Printf("Reply sent successfully to user ID: %d", originalSenderID)
		}
	} else {
		log.Println("Message is a reply but no forward information is available.")
	}
}

func (m *BotManager) DeleteBot(token string) {
	log.Printf("Attempting to delete bot with token: %s", token)
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.bots, token)
	delete(m.creator, token)
	log.Printf("Bot with token %s removed from manager's in-memory storage.", token)

	_, err := m.db.Exec("DELETE FROM bots WHERE token = ?", token)
	if err != nil {
		log.Printf("Failed to delete bot with token %s from database: %v", token, err)
	} else {
		log.Printf("Bot with token %s deleted from the database successfully.", token)
	}
}

func main() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatalf("Error loading .env file: %v", err)
	// }

	managerToken := os.Getenv("MANAGER_BOT_TOKEN")
	managerBot, err := tgbotapi.NewBotAPI(managerToken)
	if err != nil {
		log.Fatalf("Failed to create manager bot: %s", err)
	}
	log.Println("Manager bot created successfully.")

	db, err := sql.Open("sqlite", "data/bots.db")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Database connection established.")

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS bots (
	token TEXT PRIMARY KEY,
	creator_id INTEGER,
	blocked_users TEXT DEFAULT "",
	appeal_counts TEXT DEFAULT ""
   )`)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("Database table 'bots' created or already exists.")

	manager := NewBotManager(db)
	log.Println("Bot manager initialized.")

	// Load existing bots from database
	log.Println("Loading existing bots from the database...")
	rows, err := db.Query("SELECT token, creator_id FROM bots")
	if err != nil {
		log.Fatalf("Failed to load bots: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var token string
		var creatorID int64
		if err := rows.Scan(&token, &creatorID); err != nil {
			log.Fatalf("Failed to scan bot row: %v", err)
		}
		if err := manager.AddBot(token, creatorID); err != nil {
			log.Printf("Failed to add bot from database: %v", err)
		} else {
			log.Printf("Bot with token %s loaded from database and added to the manager.", token)
		}
	}
	log.Println("Existing bots loaded from database.")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := managerBot.GetUpdatesChan(u)
	log.Println("Manager bot started listening for updates.")

	for update := range updates {
		if update.Message != nil && update.Message.IsCommand() {
			log.Printf("Received a command: %s from user ID: %d in chat ID: %d", update.Message.Command(), update.Message.From.ID, update.Message.Chat.ID)
			args := update.Message.CommandArguments()
			switch update.Message.Command() {
			case "newbot":
				if err := manager.AddBot(args, update.Message.Chat.ID); err != nil {
					log.Printf("Failed to create new bot using command from user ID: %d, error: %v", update.Message.From.ID, err)
					managerBot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Failed to create new bot: "+err.Error()))
				} else {
					managerBot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "New bot created successfully!"))
					log.Printf("New bot created successfully using command from user ID: %d", update.Message.From.ID)
				}
			case "deletebot":
				manager.DeleteBot(args)
				managerBot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Bot deleted successfully!"))
				log.Printf("Bot deleted successfully using command from user ID: %d", update.Message.From.ID)
			}
		}
	}
}
