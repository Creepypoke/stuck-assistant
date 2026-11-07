# Langfuse для backend

Локальный Langfuse 4.75.0: http://localhost:1488. Проект **Backend evaluation**
создаётся автоматически. Логин и пароль — `LANGFUSE_EMAIL` и `LANGFUSE_PASSWORD`
в локальном `.env`. Файл создаётся с правами 0600, исключён из Git; повторный
`setup.py` его не перезаписывает. Нужны Docker Desktop и Python 3.9+, для новых
прогонов — Go из `backend/go.mod` и запущенные llama.cpp-серверы.

## Запуск и быстрая проверка

Из корня проекта:

```sh
make -C langfuse up
make -C langfuse baseline
make -C langfuse verify
```

`baseline` загружает **реальные существующие** P0-результаты 2B и 4B и ручные
оценки из `backend/evaluation/baselines/p0-2026-11-04/`. Модели при импорте не
вызываются. Полные результаты нужны в `backend/tmp/p0-2026-11-04/`; на этой
машине они уже есть. Если переносишь установку, перенеси эти артефакты или
получи новые командой ниже. `verify` читает кейсы, gold и каждую оценку обратно
из API и проверяет полноту загрузки (ожидает ingestion до 30 секунд).

Открой проект → **Experiments**, выбери `baseline-2b / hybrid / default` и
`baseline-4b / hybrid / default` → **Compare**. У каждого эксперимента 20 кейсов;
ещё два эксперимента содержат профиль `strict-ru`. В строках видны gold и
ответы; по кейсу можно открыть trace. В trace:

- `query.Service`: вопрос, итоговый ответ, diagnosis, полный manifest;
- `embedding`, `retrieval`, `context`: длительности, кандидаты и выбранные чанки;
- `completion-1`, при retry — `completion-2`: реальные messages с system prompt,
  модель, параметры, finish reason и usage, если его вернул сервер.

`gold_strict_pass` — импортированная ручная оценка правильности ответа,
с пояснением и рубрикой. `candidate_*` / `context_*` — наличие нужных evidence,
`runtime_success` — отсутствие ошибки исполнения. У новых прогонов gold-оценка
сама не появляется: оцени ответы в UI или передай проверенный `--review`.
Попадание нужного документа не считается правильным ответом.
Необязательная ручная оценка `answer_language_match` отдельно показывает,
ответила ли модель на языке вопроса; правильность фактов она не заменяет.

## Новый прогон текущего backend

Запусти штатные llama.cpp-серверы генерации и embeddings. Пример для отдельных
серверов на портах 18081 и 18082; при других адресах измени флаги:

```sh
python3 langfuse/pipeline.py run \
  --name my-2b-v1 -- \
  --embedding-url http://127.0.0.1:18082 \
  --llama-url http://127.0.0.1:18081 --model assistance-2b

```

Обёртка запускает `go run ./cmd/prompt-eval` из `backend`, затем экспортирует
его артефакты и проверяет их через API Langfuse. После успешной проверки
временный каталог удаляется; при ошибке он остаётся в игнорируемом
`backend/bin/evaluation-artifacts/` для повторного импорта. Это настоящий `query.Service` с текущими промптами, retrieval и
retry, без дублирования RAG-логики на Python. Daemon поднимать не требуется.
Пути после `--` считаются от `backend`; `--out` — от текущего каталога shell.
Штатные параметры: 20 вопросов, профили `default,strict-ru`, поиск `hybrid`.
Доступны `--profiles`, `--modes`, `--cases`, `--corpus`, `--seed` и остальные
флаги evaluator. При необходимости сохранить локальные артефакты укажи `--out` в игнорируемом Git каталоге; для новой конфигурации используй новый каталог.

В runner есть строгая проверка `/props`, хешей GGUF и Jinja-шаблона с
`enable_thinking`. Серверы должны предоставлять эти данные, а файлы весов быть
доступны Go-процессу. Подробнее: [протокол P0](../backend/evaluation/README.md).

Для сравнения 4B повтори команду с её endpoint/model и новым `--name`.
Для проверки генерации на вручную подтверждённых источниках добавь
`--kind oracle` **до** разделителя `--`; для проверки только retrieval —
`--kind retrieval`.

Если Langfuse был выключен или нужна повторная отправка:

```sh
python3 langfuse/pipeline.py import backend/bin/evaluation-artifacts/<каталог-из-вывода> --name my-2b-v1
```

Повторный импорт с тем же именем и неизменными артефактами использует те же
ID экспериментов, trace и scores. Изменение результатов/manifest создаёт новую
версию. Ручной review принимается только при совпадении ответа, gold и хешей
контекста/промпта. Ошибочные кейсы тоже экспортируются; неуспешный backend-прогон
сохраняет ненулевой exit code.

## Как это устроено и границы

`Go evaluator → manifest/questions/results → pipeline.py → Langfuse`.
Датасет версионируется хешем вопросов; каждый режим поиска и профиль — отдельный
эксперимент. В metadata сохраняются модель, seed, хеш корпуса/manifest; внутри
manifest также точные веса, runtime и исходники. Передача — OTLP/HTTP JSON и
публичные API Langfuse, Python-пакеты устанавливать не нужно.

Это **offline evaluation** query-логики. Обычные запросы daemon `/ask` автоматически
не трассируются, HTTP/SSE-обвязка этим прогоном не проверяется. По умолчанию
backend отправляет `enable_thinking=false`; evaluator может явно включить
thinking и изменить бюджет/temperature. Новые traces сохраняют контент каждой
попытки, в том числе усечённой, и reasoning_content в metadata completion.
Usage output включает все сгенерированные токены; отдельное reasoning_tokens
передаётся только при наличии в ответе сервера. Исторические P0-артефакты этих
текстов/полей не содержат.

Временная шкала восстанавливается из измеренных длительностей, с датой импорта;
это не исходное wall-clock время запросов. Embeddings в P0 кешируются, поэтому
тайминги не являются production latency. Seed не гарантирует идентичные ответы.

Compose содержит web/worker, PostgreSQL, ClickHouse, Redis и S3-хранилище
SeaweedFS 4.52 (официальные образы MinIO оказались недоступны). Наружу открыты
только localhost:1488 и localhost:9090. Данные живут в Docker volumes; они
сохраняются при `make -C langfuse down`. Не добавляй `down -v`, если данные нужны.

Диагностика и тесты:

```sh
make -C langfuse status
make -C langfuse test
docker compose -f langfuse/compose.yaml logs --tail 50 langfuse-web langfuse-worker
```

Основа конфигурации: [официальный Docker Compose](https://langfuse.com/self-hosting/deployment/docker-compose),
[эксперименты через OTEL](https://langfuse.com/integrations/native/opentelemetry/experiments).
