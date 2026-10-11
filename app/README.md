# Рассыльщик (Bulk SMS Campaign Simulator)

Демо-приложение для хакатона: имитация массовых рассылок сообщений через PostgreSQL с поддержкой метрик Prometheus и горизонтального автоскейлинга KEDA (scale-to-zero).

## Компоненты

1. **notifier-api**: HTTP REST API над PostgreSQL. Создает кампании, генерирует сообщения (`generate_series`) в статусе `pending`, отдает статистику и метрики Prometheus (`notifier_queue_pending`).
2. **notifier-worker**: Воркер очереди. Забирает пачки сообщений через `SELECT ... FOR UPDATE SKIP LOCKED`, симулирует отправку, фиксирует результаты (`sent`/`failed`). Корректно завершает текущую пачку по SIGTERM.
3. **loadgen**: Утилита генерации нагрузки. Поддерживает создание кампаний (`loadgen campaign`) и запуск HTTP-бенчмарка с замером RPS и перцентилей задержек (`loadgen http`).

## Переменные окружения

### notifier-api
- `DATABASE_URL`: Строка подключения к PostgreSQL (обязательно).
- `HTTP_ADDR`: Сетевой адрес и порт HTTP API (по умолчанию `:8080`).
- `APP_VERSION`: Версия приложения (по умолчанию берется из флагов сборки или `dev`).
- `FAULT_ERROR_RATE`: Вероятность искусственной ошибки 500 для маршрутов `/api/` (диапазон `0..1`, по умолчанию `0`).
- `FAULT_LATENCY_MS`: Искусственная задержка для маршрутов `/api/` в миллисекундах (по умолчанию `0`).

### notifier-worker
- `DATABASE_URL`: Строка подключения к PostgreSQL (обязательно).
- `HTTP_ADDR`: Адрес для метрик Prometheus и проверки здоровья (по умолчанию `:9090`).
- `APP_VERSION`: Версия воркера (по умолчанию `dev`).
- `BATCH_SIZE`: Размер пачки сообщений за одну транзакцию (по умолчанию `100`).
- `BATCH_TIMEOUT`: Таймаут обработки одной пачки (по умолчанию `30s`).
- `SEND_LATENCY_MS`: Задержка на отправку одного сообщения в миллисекундах (по умолчанию `20`).
- `SEND_FAIL_RATE`: Вероятность сбоя отправки (по умолчанию `0.01`).
- `MAX_ATTEMPTS`: Максимальное число попыток до перевода в статус `failed` (по умолчанию `3`).
- `POLL_INTERVAL`: Интервал опроса при пустой очереди (по умолчанию `1s`).

## Маршруты API

- `POST /api/v1/campaigns` — создать кампанию `{"text":"...", "recipients": 1000}`.
- `GET /api/v1/campaigns/{id}` — получить статус кампании и количество сообщений по статусам.
- `GET /api/v1/campaigns` — получить последние 20 кампаний.
- `GET /healthz` — проверка живучести (200 OK).
- `GET /readyz` — проверка готовности (проверка пинга базы, 503 при отсутствии связи).
- `GET /metrics` — метрики Prometheus.
- `GET /version` — текущая версия сервиса.

## Метрики Prometheus

- `http_requests_total{method,route,code,version}`
- `http_request_duration_seconds{route,version}`
- `notifier_campaigns_created_total`
- `notifier_queue_pending`
- `notifier_messages_processed_total{result="sent|failed|retry"}`
- `notifier_batch_duration_seconds`
- `notifier_batch_size`
- `notifier_build_info{version}`

## Локальный запуск

```bash
docker compose up -d
```

## Генерация нагрузки

```bash
# Создание кампании на 50 000 сообщений
./loadgen campaign -url http://localhost:8080 -recipients 50000 -text "Промо-акция"

# Открытый цикл запросов 300 RPS на 2 минуты
./loadgen http -url http://localhost:8080/api/v1/campaigns -rps 300 -duration 2m -workers 64
```
