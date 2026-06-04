package main

import (
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

	"gopkg.in/yaml.v3"
)

type Config struct {
	BotToken string `yaml:"bot_token"`
	APIKey   string `yaml:"api_key"`
	Listen   string `yaml:"listen"`
}

type GlitchTipAlert struct {
	Alias     string `json:"alias"`
	Text      string `json:"text"`
	ProjectID int    `json:"project_id"`
	Issue     struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Culprit  string `json:"culprit"`
		URL      string `json:"url"`
		Count    int    `json:"count"`
		Level    string `json:"level"`
		Metadata struct {
			Type     string `json:"type"`
			Value    string `json:"value"`
			Filename string `json:"filename"`
		} `json:"metadata"`
	} `json:"issue"`
	Project struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"project"`
	TriggerType string `json:"trigger_type"`
}

var config Config

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("failed to read config: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}
	if config.BotToken == "" {
		log.Fatal("bot_token is required")
	}
	if config.APIKey == "" {
		log.Fatal("api_key is required")
	}
	if config.Listen == "" {
		config.Listen = ":8080"
	}

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	log.Printf("listening on %s", config.Listen)
	log.Fatal(http.ListenAndServe(config.Listen, nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	key := q.Get("key")
	if subtle.ConstantTimeCompare([]byte(key), []byte(config.APIKey)) != 1 {
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

	log.Printf("sent alert to chat %d: %s", chatID, alert.Issue.Title)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func formatMessage(a GlitchTipAlert) string {
	level := strings.ToUpper(a.Issue.Level)
	if level == "" {
		level = "ERROR"
	}

	emoji := "🔴"
	switch strings.ToLower(a.Issue.Level) {
	case "warning":
		emoji = "🟡"
	case "info":
		emoji = "🔵"
	case "debug":
		emoji = "⚪"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s <b>[%s] %s</b>\n", emoji, level, escapeHTML(a.Project.Name))
	fmt.Fprintf(&b, "\n<b>%s</b>", escapeHTML(a.Issue.Title))

	hasDetails := a.Issue.Metadata.Value != "" || a.Issue.Culprit != "" || a.Issue.Count > 0
	if hasDetails {
		b.WriteString("\n\n<blockquote expandable>")
		if a.Issue.Metadata.Value != "" {
			fmt.Fprintf(&b, "\n<code>%s</code>", escapeHTML(a.Issue.Metadata.Value))
		}
		if a.Issue.Culprit != "" {
			fmt.Fprintf(&b, "\n📍 <code>%s</code>", escapeHTML(a.Issue.Culprit))
		}
		if a.Issue.Count > 0 {
			fmt.Fprintf(&b, "\n🔢 Events: %d", a.Issue.Count)
		}
		b.WriteString("\n</blockquote>")
	}

	if a.Issue.URL != "" {
		fmt.Fprintf(&b, "\n\n<a href=\"%s\">View in GlitchTip</a>", a.Issue.URL)
	}

	return b.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func sendTelegram(chatID int64, topicID *int, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)

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
