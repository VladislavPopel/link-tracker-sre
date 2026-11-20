## 1. Описание выбранных метрик

Метрики приложения собираются Victoria Metrics (интервал сбора 15 секунд) с подов по аннотациям `prometheus.io/*`.

- `linktracker_http_requests_total` - счётчик HTTP-запросов к scrapper; метки `method`, `route`, `status`.
- `linktracker_http_request_duration_seconds` - гистограмма длительности HTTP-запросов (метки `method`, `route`); для SLI используется `le="0.25"`.
- `linktracker_link_checks_total` - счётчик фоновых проверок ссылок планировщиком; метки `checker` (`github`, `stackoverflow`) и `result` (`updated`, `no_updates`, `error`).
- `linktracker_probe_requests_total` - счётчик запросов пробера (`cmd/prober`), который раз в 5 с обращается к scrapper через Service как внешний клиент: `GET /` (`probe="frontend"`) и `GET /tg-chat?limit=1` (`probe="api"`, до БД). Успех - ответ 200 за 2 с (`result="success"`), иначе `result="failure"`.

## 2. Формулировки SLI, SLO, SLA и формулы

В формулах окно W - период, за который оценивается SLO.

### SLI-1. Доступность API (RBSLO)

Доля запросов, обработанных без ошибки сервера (код ответа не 5xx).

```
SLI1 = sum(rate(linktracker_http_requests_total{status!~"5.."}[1m])) / sum(rate(linktracker_http_requests_total[1m]))
```

SLO-1: доля успешных запросов не ниже 99,5% за окно W.

```
attainment = sum(increase(linktracker_http_requests_total{status!~"5.."}[W])) / sum(increase(linktracker_http_requests_total[W]))
остаток бюджета ошибок = 1 - (1 - attainment) / (1 - 0.995)
```

Остаток бюджета равен 1, если ошибок не было, 0 - бюджет исчерпан, меньше 0 - SLO нарушен.

### SLI-2. Скорость ответа (RBSLO)

Доля запросов, обработанных быстрее 250 мс.

```
SLI2 = sum(rate(linktracker_http_request_duration_seconds_bucket{le="0.25"}[1m])) / sum(rate(linktracker_http_request_duration_seconds_count[1m]))
```

SLO-2: не менее 95% запросов обрабатываются быстрее 250 мс.

### SLI-3. Успешность проверок ссылок (RBSLO)

Доля проверок ссылок, завершившихся без ошибки.

```
SLI3 = sum(rate(linktracker_link_checks_total{result!="error"}[5m])) / sum(rate(linktracker_link_checks_total[5m]))
```

SLO-3: не менее 95% проверок завершаются без ошибки.

### SLI-4. Доступность по пробе (TBSLO)

Минута считается хорошей, если за неё ни одна проба не завершилась неуспешно. SLI - доля хороших минут в окне.

```
good_minute = sum(increase(linktracker_probe_requests_total{result="failure"}[1m])) == 0
SLI4 = avg_over_time(good_minute[W:1m])
```

SLO-4: не менее 99% минут окна хорошие.

### SLA

Доступность API (доля запросов без ошибок сервера, то есть SLI-1 за окно) не ниже 99% в календарный месяц. При нарушении клиенту предоставляется компенсация.

```
SLA выполнено, если attainment >= 0.99
```

SLA мягче, чем SLO-1 (99% против 99,5%): между внутренней целью и обещанием клиенту остаётся запас.

## 3. Изменения приложения для скринкаста

Пробер. Добавлен сервис cmd/prober, он находится (k8s/30-prober.yaml). Пробер создаёт постоянный трафик и независимую оценку доступности.

Переключатели сбоев: В scrapper добавлен middleware internal/scrapper/fault.go, управляемый переменными окружения:

- `FAULT_ERROR_RATE` - доля запросов (от 0 до 1), на которые сервис отвечает ошибкой 500;
- `FAULT_DELAY_MS` - искусственная задержка каждого запроса в миллисекундах.

Ход эксперимента:

1. Нормальная работа: SLI близки к 1, бюджет ошибок не тронут, плохих минут нет.
2. `FAULT_ERROR_RATE=0.3`: SLI-1 упал до минимума 0,567, остаток бюджета SLO-1 ушёл в минус (минимум −15,6), то есть SLO нарушен; пробы начали падать, появились плохие минуты (SLI-4).
3. Ошибки отключены, `FAULT_DELAY_MS=400`: SLI-1 восстановился, SLI-2 упал до 0, так как все запросы стали медленнее 250 мс. Пробер при этом остаётся успешным (400 мс меньше его таймаута 2 секунды), поэтому SLI-4 не реагирует: ошибки и медленные ответы фиксируются разными SLI.
4. Переключатели сброшены: все SLI возвращаются к норме, SLO и бюджет восстанавливаются постепенно, потому что окно оценки содержит прошлые данные.