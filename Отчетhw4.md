## Стек

- Язык: Go, сервис **scrapper**
- БД: PostgreSQL 16
- Деплой: Kubernetes (namespace `link-tracker`), манифесты в `10-postgres.yaml`, `20-scrapper.yaml`
- Хранилище метрик: VictoriaMetrics Single (helm-чарт, namespace `monitoring`)
- Сбор метрик: service discovery по аннотациям `prometheus.io/scrape`, `prometheus.io/port`, `prometheus.io/path`

## Экспортеры

- **prometheus/client_golang**: метрики приложения, отдаются на отдельном порту `9090` по пути `/metrics`. Метрики рантайма Go и процесса (`go_*`, `process_*`) включены по умолчанию в `promhttp.Handler()`. Реализованы метрики `linktracker_http_requests_total`, `linktracker_http_request_duration_seconds`.
- **postgres-exporter**: sidecar-контейнер в поде Postgres, порт `9187`, метрики с префиксом `pg_`.

## Бизнес-метрики

| Метрика | Тип | Описание |
|---|---|---|
| `linktracker_links_added_total` | Counter | число добавленных ссылок |
| `linktracker_links_removed_total` | Counter | число удалённых ссылок |
| `linktracker_link_checks_total{checker,result}` | Counter | проверки ссылок по источнику (`github`, `stackoverflow`) и результату (`updated`, `no_updates`, `error`) |
| `linktracker_notifications_created_total` | Counter | число созданных уведомлений |
| `linktracker_check_cycle_duration_seconds` | Histogram | длительность одного цикла проверки всех ссылок |

## Аннотации

Аннотации указаны в `spec.template.metadata` подов:

- scrapper: порт `9090`, путь `/metrics`
- postgres: порт `9187`, путь `/metrics`