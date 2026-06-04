package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type AttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type Attachment struct {
	Title     string            `json:"title"`
	TitleLink string            `json:"title_link"`
	Text      string            `json:"text"`
	ImageURL  *string           `json:"image_url"`
	Color     string            `json:"color"`
	Fields    []AttachmentField `json:"fields"`
	MrkdwnIn  []string          `json:"mrkdown_in"`
}

type GlitchTipAlert struct {
	Alias       string       `json:"alias"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

var (
	botToken string
	apiKey   string
	port     string
)

func main() {
	loadEnv(".env")

	botToken = getEnv("BOT_TOKEN", "")
	apiKey = getEnv("API_KEY", "")
	port = getEnv("PORT", "8080")

	if botToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}
	if apiKey == "" {
		log.Fatal("API_KEY is required")
	}

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	key := q.Get("key")
	if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	chatIDStr := q.Get("chat_id")
	if chatIDStr == "" {
		http.Error(w, "chat_id is required", http.StatusBadRequest)
		return
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid chat_id", http.StatusBadRequest)
		return
	}

	var topicID *int
	if t := q.Get("topic_id"); t != "" {
		v, err := strconv.Atoi(t)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		topicID = &v
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("failed to read body: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var alert GlitchTipAlert
	if err := json.Unmarshal(body, &alert); err != nil {
		log.Printf("failed to parse alert: %v", err)
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	msg := formatMessage(alert)
	if err := sendTelegram(chatID, topicID, msg); err != nil {
		log.Printf("failed to send to telegram: %v", err)
		http.Error(w, "telegram error", http.StatusBadGateway)
		return
	}

	title := alert.Text
	if len(alert.Attachments) > 0 {
		title = alert.Attachments[0].Title
	}
	log.Printf("sent alert to chat %d: %s", chatID, title)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func formatMessage(a GlitchTipAlert) string {
	if len(a.Attachments) == 0 {
		return escapeHTML(a.Text)
	}

	var b strings.Builder

	for i, att := range a.Attachments {
		if i > 0 {
			b.WriteString("\n\n───────────────\n\n")
		}

		emoji := emojiForColor(att.Color)

		project := fieldValue(att.Fields, "Project")
		if project != "" {
			fmt.Fprintf(&b, "%s <b>[%s]</b>\n", emoji, escapeHTML(project))
		} else {
			fmt.Fprintf(&b, "%s ", emoji)
		}

		if att.TitleLink != "" {
			fmt.Fprintf(&b, "\n<a href=\"%s\">%s</a>", att.TitleLink, escapeHTML(att.Title))
		} else {
			fmt.Fprintf(&b, "\n<b>%s</b>", escapeHTML(att.Title))
		}

		hasDetails := att.Text != "" || len(att.Fields) > 0
		if hasDetails {
			b.WriteString("\n\n<blockquote expandable>")
			if att.Text != "" {
				fmt.Fprintf(&b, "\n📍 <code>%s</code>", escapeHTML(att.Text))
			}
			for _, f := range att.Fields {
				if strings.EqualFold(f.Title, "Project") {
					continue
				}
				fmt.Fprintf(&b, "\n%s: %s", escapeHTML(f.Title), escapeHTML(f.Value))
			}
			b.WriteString("\n</blockquote>")
		}
	}

	if len(a.Attachments) > 1 {
		fmt.Fprintf(&b, "\n\n📊 %d issues total", len(a.Attachments))
	}

	return b.String()
}

func emojiForColor(color string) string {
	switch strings.ToLower(color) {
	case "#e9b949":
		return "🟡"
	case "#4b60b4":
		return "🔵"
	case "#e52b50":
		return "🔴"
	default:
		return "⚪"
	}
}

func fieldValue(fields []AttachmentField, title string) string {
	for _, f := range fields {
		if strings.EqualFold(f.Title, title) {
			return f.Value
		}
	}
	return ""
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func sendTelegram(chatID int64, topicID *int, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	params := url.Values{}
	params.Set("chat_id", fmt.Sprintf("%d", chatID))
	params.Set("text", text)
	params.Set("parse_mode", "HTML")
	params.Set("disable_web_page_preview", "true")

	if topicID != nil {
		params.Set("message_thread_id", fmt.Sprintf("%d", *topicID))
	}

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
