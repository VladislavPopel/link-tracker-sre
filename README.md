# LinkTracker

**LinkTracker** – Telegram-бот, который отслеживает изменения на веб-страницах и оперативно информирует пользователя о них.


Это шаблон проекта, который вам необходимо взять за основу для разработки своей системы.


В данном файле должна находиться инструкция для ассистента по запуску и настройке бота.
Полезную для разработки проекта информацию вы можете найти в файле [HELP.md.](./HELP.md)


# Инструкция по запуску

Заполнить в файле .env:
- `DATABASE_DSN` — строка подключения к PostgreSQL 
(`postgres://postgres:postgres@localhost:5432/link_tracker?sslmode=disable` по умолчанию) 
- `DATABASE_ACCESS_TYPE` — `SQL` или `ORM` 
(`SQL` по умолчанию)

1. Запуск PostgreSQL:
    `docker compose up -d`
2. Запустить 2 команды:
    `$env:TELEGRAM_APITOKEN="сюда_вставить_токен"; go run cmd/bot/main.go` (**PowerShell**) 
    `go run ./cmd/scrapper`
    (в разработке использовалась данная команда)
    `set "TELEGRAM_APITOKEN=сюда_вставить_токен" && go run cmd/bot/main.go` (**CMD**)
    `TELEGRAM_APITOKEN="сюда_вставить_токен" go run cmd/bot/main.go` (**Linux/macOS**)

# или

1. Создать файл .env в директории проекта
2. Добавить в файл .env строку `TELEGRAM_APITOKEN="сюда_вставить_токен"`
3. Установить пакет godotenv для парсинга файла .env
4. `go run cmd/bot/main.go` и `go run ./cmd/scrapper`

# Запуск тестов

`go test ./...` - запуск всех тестов
    Запуск unit-тестов:
`go test ./internal/application/...`
`go test ./internal/bot/...`
