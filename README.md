# glitchtip-telegram

Lightweight Go bridge that forwards [GlitchTip](https://glitchtip.com) webhook alerts to Telegram chats. Zero external dependencies, single binary.

## How it works

```
GlitchTip ──POST──▶ bridge ──▶ Telegram Bot API ──▶ chat/topic
```

Chat ID and topic ID are passed as query parameters in the webhook URL, so you can route any number of GlitchTip projects to different Telegram chats without touching the bridge config.

## Setup

### 1. Create a Telegram bot

Message [@BotFather](https://t.me/BotFather), create a bot, and copy the token.

### 2. Get your chat IDs

Add the bot to your group(s), send a message, then check:

```
https://api.telegram.org/bot<TOKEN>/getUpdates
```

Look for `chat.id` (negative number for groups). For forum/supergroup topics, note the `message_thread_id`.

### 3. Configure

```bash
cp .env.sample .env
```

```env
BOT_TOKEN=123456:ABC-DEF...
API_KEY=any-secret-you-choose
PORT=8080
```

### 4. Run

**Docker Compose:**

```bash
docker compose up -d
```

**Or build directly:**

```bash
go build -o glitchtip-telegram .
./glitchtip-telegram
```

### 5. Add webhook in GlitchTip

Go to **Project Settings > Alerts > Add Alert** and add a webhook notification with the URL:

```
http://your-server:8080/webhook?key=YOUR_API_KEY&chat_id=-100123456789
```

With a topic:

```
http://your-server:8080/webhook?key=YOUR_API_KEY&chat_id=-100123456789&topic_id=42
```

## Query parameters

| Parameter  | Required | Description                          |
|------------|----------|--------------------------------------|
| `key`      | yes      | Must match `API_KEY` from `.env`     |
| `chat_id`  | yes      | Telegram chat ID                     |
| `topic_id` | no       | Forum/supergroup topic thread ID     |

## Health check

```
GET /health → 200 ok
```

## License

MIT
