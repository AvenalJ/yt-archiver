package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AvenalJ/yt-archiver/internal/logger"
)

type LiveChatMessage struct {
	Author        string `json:"author"`
	AuthorPhoto   string `json:"author_photo"`
	Message       string `json:"message"`
	TimeOffsetMs  int64  `json:"time_offset_ms"`
	TimeFormatted string `json:"time_formatted"`
	Badge         string `json:"badge,omitempty"`
	Superchat     string `json:"superchat,omitempty"`
}

// ProcessLiveChatFile scans for .live_chat.json in outputDir and parses it into structured live_chat.json
func ProcessLiveChatFile(outputDir string) []LiveChatMessage {
	if outputDir == "" {
		return nil
	}

	// Look for any file ending with .live_chat.json
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil
	}

	var rawChatFile string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".live_chat.json") {
			rawChatFile = filepath.Join(outputDir, entry.Name())
			break
		}
	}

	if rawChatFile == "" {
		return nil
	}

	file, err := os.Open(rawChatFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	var messages []LiveChatMessage
	scanner := bufio.NewScanner(file)
	// Larger buffer for massive chat JSON lines
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var root map[string]interface{}
		if err := json.Unmarshal([]byte(line), &root); err != nil {
			continue
		}

		rcia, ok := root["replayChatItemAction"].(map[string]interface{})
		if !ok {
			continue
		}

		offsetMs := int64(0)
		if offsetStr, ok := rcia["videoOffsetTimeMsec"].(string); ok {
			if ms, err := strconv.ParseInt(offsetStr, 10, 64); err == nil {
				offsetMs = ms
			}
		}

		actions, ok := rcia["actions"].([]interface{})
		if !ok {
			continue
		}

		for _, act := range actions {
			actMap, ok := act.(map[string]interface{})
			if !ok {
				continue
			}

			if addChat, ok := actMap["addChatItemAction"].(map[string]interface{}); ok {
				if item, ok := addChat["item"].(map[string]interface{}); ok {
					msg := parseChatItem(item, offsetMs)
					if msg != nil {
						messages = append(messages, *msg)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Warnf("[LiveChat] Scanner encountered error reading chat: %v", err)
	}

	if len(messages) == 0 {
		return nil
	}

	// Sort messages chronologically
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].TimeOffsetMs < messages[j].TimeOffsetMs
	})

	// Save clean structured live_chat.json
	cleanPath := filepath.Join(outputDir, "live_chat.json")
	if jsonBytes, err := json.MarshalIndent(messages, "", "  "); err == nil {
		_ = os.WriteFile(cleanPath, jsonBytes, 0644)
	}

	logger.Infof("[LiveChat] Processed %d replay messages for %s", len(messages), filepath.Base(outputDir))
	return messages
}

func parseChatItem(item map[string]interface{}, offsetMs int64) *LiveChatMessage {
	// Standard Text Message
	if textRenderer, ok := item["liveChatTextMessageRenderer"].(map[string]interface{}); ok {
		author := ""
		if aObj, ok := textRenderer["authorName"].(map[string]interface{}); ok {
			if s, ok := aObj["simpleText"].(string); ok {
				author = s
			}
		}
		photo := ""
		if pObj, ok := textRenderer["authorPhoto"].(map[string]interface{}); ok {
			if thumbs, ok := pObj["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
				if t0, ok := thumbs[0].(map[string]interface{}); ok {
					if u, ok := t0["url"].(string); ok {
						photo = u
					}
				}
			}
		}
		msgText := ""
		if mObj, ok := textRenderer["message"].(map[string]interface{}); ok {
			if runs, ok := mObj["runs"].([]interface{}); ok {
				var sb strings.Builder
				for _, r := range runs {
					if rMap, ok := r.(map[string]interface{}); ok {
						if t, ok := rMap["text"].(string); ok {
							sb.WriteString(t)
						} else if emoji, ok := rMap["emoji"].(map[string]interface{}); ok {
							if shortcuts, ok := emoji["shortcuts"].([]interface{}); ok && len(shortcuts) > 0 {
								if sc, ok := shortcuts[0].(string); ok {
									sb.WriteString(sc)
								}
							}
						}
					}
				}
				msgText = sb.String()
			}
		}

		sec := offsetMs / 1000
		m := sec / 60
		s := sec % 60
		timeFormatted := fmt.Sprintf("%d:%02d", m, s)

		if author != "" && msgText != "" {
			return &LiveChatMessage{
				Author:        author,
				AuthorPhoto:   photo,
				Message:       msgText,
				TimeOffsetMs:  offsetMs,
				TimeFormatted: timeFormatted,
			}
		}
	}

	// Superchat / Paid Message
	if paidRenderer, ok := item["liveChatPaidMessageRenderer"].(map[string]interface{}); ok {
		author := ""
		if aObj, ok := paidRenderer["authorName"].(map[string]interface{}); ok {
			if s, ok := aObj["simpleText"].(string); ok {
				author = s
			}
		}
		amount := ""
		if pObj, ok := paidRenderer["purchaseAmountText"].(map[string]interface{}); ok {
			if s, ok := pObj["simpleText"].(string); ok {
				amount = s
			}
		}
		msgText := ""
		if mObj, ok := paidRenderer["message"].(map[string]interface{}); ok {
			if runs, ok := mObj["runs"].([]interface{}); ok {
				var sb strings.Builder
				for _, r := range runs {
					if rMap, ok := r.(map[string]interface{}); ok {
						if t, ok := rMap["text"].(string); ok {
							sb.WriteString(t)
						}
					}
				}
				msgText = sb.String()
			}
		}

		sec := offsetMs / 1000
		m := sec / 60
		s := sec % 60
		timeFormatted := fmt.Sprintf("%d:%02d", m, s)

		if author != "" {
			return &LiveChatMessage{
				Author:        author,
				Message:       msgText,
				TimeOffsetMs:  offsetMs,
				TimeFormatted: timeFormatted,
				Superchat:     amount,
			}
		}
	}

	return nil
}
