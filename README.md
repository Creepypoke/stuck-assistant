# Walkthrough Assistance Backend

Локальный backend состоит из двух отдельных программ:

- `walkthrough-assistance-service` - пользовательский daemon, который запускает
  `llama.cpp` и предоставляет локальный API для Decky-плагина;
- `walkthrough-assistance` - CLI для добавления прохождений и управления
  сервисом.

Команда добавления прохождения:

1. загружает HTML-страницу прохождения по URL или читает локальный `.html`, `.htm` либо `.txt` файл;
2. извлекает основной текст и преобразует его в Markdown;
3. разбивает документ на смысловые chunks;
4. получает embeddings через OpenAI-compatible API `llama.cpp`;
5. сохраняет исходный документ, Markdown, chunks, embeddings и FTS5-индекс в
   отдельный SQLite `.rag` файл игры.

```text
walkthrough-assistance knowledge add <game-id> <url-or-file>
```

Управление daemon:

```text
walkthrough-assistance service install|status|start|stop|restart|enable|disable
```

## Требования

- Go версии, указанной в `go.mod` (сейчас Go 1.25 или новее);
- установленный в `PATH` executable `llama` из `llama.cpp`; сервис запускает
  `llama serve` самостоятельно;
- HTML parser одним из двух способов (SteamOS-установка уже включает standalone
  parser):
  - собранный executable `walkthrough-parser`;
  - Python 3 с зависимостями из `parser/requirements.txt`.

Embedding server по умолчанию ожидается по адресу
`http://127.0.0.1:8081`. Модель по умолчанию называется
`embedding`, а размер embedding равен `768`. Эти значения можно
переопределить флагами CLI и необходимо согласовать с фактически запущенной
моделью.

## Сборка

Из корня проекта выполните:

```bash
make build
```

Команда создаёт два бинарника:

```text
bin/walkthrough-assistance
bin/walkthrough-assistance-service
```

Проверить собранный CLI:

```bash
./bin/walkthrough-assistance --help
./bin/walkthrough-assistance knowledge add --help
```

Для запуска без предварительной сборки можно использовать:

```bash
go run ./cmd/walkthrough-assistance --help
```

Daemon без установки можно запустить так:

```bash
make run-service
```

## Установка на SteamOS

Установка выполняется только в home-каталог текущего пользователя и не
изменяет read-only системную часть SteamOS. Перед первой установкой из свежего
клона соберите standalone parser: каталог `parser/dist/` с этим артефактом не
хранится в Git.

```bash
bash parser/build.sh
make install
```

Для сборки parser требуется Docker Buildx. Если standalone parser уже собран,
достаточно выполнить только `make install`.

Файлы устанавливаются в
`~/.local/share/walkthrough-assistance`, а unit - в
`~/.config/systemd/user/walkthrough-assistance.service`. Команда сразу
выполняет `systemctl --user enable --now`, поэтому сервис запускается вместе с
пользовательской сессией SteamOS и перезапускается после аварийного завершения.
Standalone parser устанавливается как вспомогательный executable рядом с CLI.

Управление после установки:

```bash
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service status
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service stop
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service start
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service restart
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service disable
~/.local/share/walkthrough-assistance/bin/walkthrough-assistance service enable
```

Для ручной установки unit либо нестандартного размещения бинарника:

```bash
walkthrough-assistance service install \
  --service-binary /absolute/path/to/walkthrough-assistance-service \
  --environment-file /absolute/path/to/.env
```

## Подготовка parser

### Вариант 1: Python

Создайте виртуальное окружение и установите зависимости:

```bash
python3 -m venv .venv
./.venv/bin/python -m pip install -r parser/requirements.txt
```

`make build` копирует `.env.example` в `bin/.env`. Для локальной разработки и
тестов standalone executable собирать не требуется: достаточно, чтобы Python
и файл parser были доступны по путям, заданным в `WALKTHROUGH_PARSER` и
`WALKTHROUGH_PARSER_ARGS`. Значения по умолчанию в `bin/.env` рассчитаны на
виртуальное окружение внутри `parser/`:

```dotenv
WALKTHROUGH_PARSER=../parser/.venv/bin/python
WALKTHROUGH_PARSER_ARGS=../parser/walkthrough_parser.py
```

Пути разрешаются относительно каталога `bin/`, поэтому после создания
`parser/.venv` по командам выше дополнительная настройка не нужна. При другом
расположении Python или parser измените эти параметры либо передайте их
флагами:

```bash
--parser ./.venv/bin/python \
--parser-arg parser/walkthrough_parser.py
```

Python используется только во время ingestion. После сохранения `.rag` файла
он для чтения базы знаний не требуется.

### Вариант 2: standalone executable

Для сборки Linux executable требуется Docker Buildx:

```bash
bash parser/build.sh
```

Артефакт для Linux AMD64 будет сохранён как:

```text
parser/dist/walkthrough-parser-linux-amd64
```

Его можно явно передать через `--parser`:

```bash
--parser ./parser/dist/walkthrough-parser-linux-amd64
```

Если executable установлен в `PATH` под именем `walkthrough-parser`, флаг
`--parser` можно не указывать.

Подробнее о сборке parser для других архитектур написано в
[`parser/README.md`](parser/README.md).

## Embedding server

`walkthrough-assistance-service` запускает установленный на host-системе
`llama.cpp` с настройками модуля [`llama-cpp`](llama-cpp/README.md). По
умолчанию выполняется эквивалент этой команды:

```bash
llama serve --host 127.0.0.1 --port 8081 \
  --models-preset llama-cpp/models.ini --models-max 1
```

Preset использует `embeddinggemma-300M` для embeddings и содержит Qwen 2B для
генерации. При аварийном завершении `llama.cpp` daemon завершается с ошибкой,
после чего systemd перезапускает весь сервис.

CLI должен знать:

- адрес сервера: `--embedding-url`;
- имя модели: `--embedding-model`;
- версию модели: `--embedding-version`;
- размер вектора: `--embedding-dimensions`.

Имя, версия и размер сохраняются в metadata `.rag` файла. Повторная запись в
тот же файл с несовместимыми параметрами будет отклонена.

## Использование

`game-id` должен быть стабильным slug в нижнем регистре. Допустимы латинские
буквы, цифры и дефисы, например `elden-ring` или `witcher-3-wild-hunt`.

Пример запуска с Python parser:

```bash
./bin/walkthrough-assistance knowledge add \
  elden-ring \
  https://example.com/elden-ring-walkthrough
```

Для создания RAG-артефакта без скачивания передайте путь к локальному файлу:

```bash
./bin/walkthrough-assistance knowledge add \
  elden-ring \
  ./fixtures/elden-ring-guide.html
```

Поддерживаются только обычные файлы с расширениями `.html`, `.htm` и `.txt`.

То же без предварительной сборки backend:

```bash
go run ./cmd/walkthrough-assistance knowledge add \
  elden-ring \
  https://example.com/elden-ring-walkthrough
```

При успешном завершении команда выводит результат:

```text
document_id: 1
chunks: 42
path: /home/user/.local/share/walkthrough-assistance/games/elden-ring.rag
```

По умолчанию файлы сохраняются в:

```text
~/.local/share/walkthrough-assistance/games/<game-id>.rag
```

Другой каталог можно указать через `--storage-dir`:

```bash
./walkthrough-assistance knowledge add \
  elden-ring \
  https://example.com/elden-ring-walkthrough \
  --storage-dir ./data/games \
  --parser ./.venv/bin/python \
  --parser-arg parser/walkthrough_parser.py
```

## Конфигурация

Собранный CLI автоматически загружает `bin/.env`. Он создаётся из
`.env.example` командой `make build`; измените его значения перед запуском.
Переменные окружения имеют приоритет над значениями из файла, флаги CLI - над
обоими. Поддерживаются:

```text
WALKTHROUGH_GAME_NAME
WALKTHROUGH_PARSER
WALKTHROUGH_PARSER_ARGS              Аргументы через пробел
WALKTHROUGH_STORAGE_DIR
WALKTHROUGH_EMBEDDING_URL
WALKTHROUGH_EMBEDDING_MODEL
WALKTHROUGH_EMBEDDING_VERSION
WALKTHROUGH_EMBEDDING_DIMENSIONS
WALKTHROUGH_CHUNK_TOKENS
WALKTHROUGH_OVERLAP_TOKENS
WALKTHROUGH_DOWNLOAD_TIMEOUT
WALKTHROUGH_EMBEDDING_TIMEOUT
WALKTHROUGH_LISTEN_ADDRESS            По умолчанию 127.0.0.1:19273
WALKTHROUGH_SERVICE_URL               По умолчанию http://127.0.0.1:19273
WALKTHROUGH_LLAMA_COMMAND             По умолчанию llama
WALKTHROUGH_LLAMA_ARGS                Полная замена аргументов llama через пробел
WALKTHROUGH_LLAMA_MODELS_PRESET       Путь к models.ini
WALKTHROUGH_LLAMA_HEALTH_URL          По умолчанию http://127.0.0.1:8081/health
WALKTHROUGH_LLM_URL                   По умолчанию http://127.0.0.1:8081
WALKTHROUGH_LLM_MODEL                 По умолчанию assistance
WALKTHROUGH_LLM_API_KEY               Необязательный Bearer-токен только для LLM
WALKTHROUGH_LLM_TIMEOUT                По умолчанию 10m
WALKTHROUGH_SYSTEM_PROMPT_FILE          Путь к UTF-8 файлу с системным промптом daemon
```

## API для Decky

API слушает только loopback-интерфейс. Текущие endpoint:

```text
GET http://127.0.0.1:19273/health
GET http://127.0.0.1:19273/api/v1/status
```

`/health` возвращает `503`, пока `llama.cpp` не готов принимать запросы.
`/api/v1/status` всегда возвращает состояние daemon и дочернего процесса в
JSON. Для чтения API из Decky UI добавлен CORS-заголовок.

Для локального тестирования запроса из Decky на macOS используйте CLI:

```bash
./bin/walkthrough-assistance ask dino-crisis-psx "Where do I find the key?" \
  --display-name "Dino Crisis"
```

Для диагностики добавьте `--log-level debug`. Ответ будет содержать SSE-событие
`debug` с исходным запросом, метаданными, всеми выбранными RAG-фрагментами и
точным JSON payload, переданным LLM:

```bash
./bin/walkthrough-assistance --log-level debug ask dino-crisis-psx "Where do I find the key?"
```

Чтобы сравнить ответ модели без контекста прохождения, используйте `--no-rag`:

```bash
./bin/walkthrough-assistance --log-level debug ask dino-crisis-psx "Куда идти в начале игры?" --no-rag
```

В этом режиме сервис не обращается к embeddings и `.rag`, не выдаёт события
`source`, а в `debug` показывает `retrieved_chunks: []` и запрос к модели
без `Walkthrough context`. Ответ основан на знаниях самой модели, поэтому
может быть неточным. Флаг действует только для конкретного вызова `ask`.

Доступны уровни `debug`, `info`, `warn` и `error`; для daemon уровень также
задаётся переменной `WALKTHROUGH_LOG_LEVEL`. Debug-контекст может содержать
текст прохождения, поэтому не используйте этот режим при записи публичных логов.
Команда отправляет `POST /ask` с вопросом и каноническим локальным `game_id`.
Локация, квест и прогресс в MVP не определяются и не передаются модели;
для уточнения игрок может написать о своём положении в самом вопросе.
Для совместимости сервис игнорирует эти поля, если старый клиент отправляет их
в `context`.
`--service-url` или `WALKTHROUGH_SERVICE_URL` позволяет указать другой
локальный адрес. Ответ сервиса, включая SSE-события, передаётся в stdout.
Для медленной генерации можно задать общий лимит CLI через `--timeout 10m`;
лимит внутреннего запроса сервиса к LLM настраивается через
`WALKTHROUGH_LLM_TIMEOUT`.

### Удалённая LLM при локальных embeddings

Для OpenAI-compatible endpoint генерации укажите в `.env` **сервиса**:

```dotenv
WALKTHROUGH_LLM_URL=https://openai.home-router.crazedns.ru
WALKTHROUGH_LLM_MODEL=<имя-модели-на-сервере>
WALKTHROUGH_LLM_API_KEY=<ваш-ключ>
WALKTHROUGH_EMBEDDING_URL=http://127.0.0.1:8081
```

К базовому `WALKTHROUGH_LLM_URL` сервис добавляет `/v1/chat/completions`;
не дописывайте `/v1` в переменную. Ключ передаётся **только** в заголовке
`Authorization: Bearer ...` запросов генерации и не включается в debug-вывод.
Без ключа заголовок не отправляется. Локальная `llama.cpp` по-прежнему
запускается сервисом и обслуживает embeddings: удалённый сервер должен
поддерживать OpenAI-compatible chat completions, но не обязан поддерживать
embeddings. После изменения `.env` перезапустите сервис. Храните файл с
ключом только локально и ограничьте к нему доступ (`chmod 600 bin/.env`).
`make build` копирует `.env.example` в `bin/.env` и может перезаписать ваши
настройки — сохраните их перед пересборкой.

По умолчанию системный промпт задаёт дружелюбного помощника по прохождению:
он отвечает на языке вопроса, предлагает ближайший подтверждённый шаг и
уточняет положение игрока, если найденные фрагменты не дают надёжного ответа.
Чтобы изменить личность, стиль и тон ответов, создайте UTF-8 файл с
многострочным текстом и укажите путь через `WALKTHROUGH_SYSTEM_PROMPT_FILE` в
окружении daemon (или в его `.env`), затем перезапустите сервис. Ограничения
`--no-spoilers` и `--answer-length` добавляются к пользовательскому промпту.

Промпт не исправляет ошибки поиска: например, в локальном
`dino-crisis-psx.rag` отсутствуют названия секций, а фрагменты могут
обрываться посреди предложений. Если ответ всё ещё неточен, запустите
`ask` с `--log-level debug`, проверьте `retrieved_chunks` и `model_request`;
при низком качестве источника создайте новый RAG-файл из более качественного
прохождения в отдельном каталоге (повторный `knowledge add` в существующий файл
добавляет документ, а не заменяет старый).

Поиск учитывает явное упоминание начала игры в самом вопросе: в этом случае
берутся первые фрагменты прохождения вместо случайного похожего эпизода.
Остальные вопросы ищутся по тексту вопроса семантически.
Для коротких ответов лимит генерации увеличен до 256 токенов. Если LLM
сообщит `finish_reason=length`, запрос будет повторён один раз с удвоенным
лимитом; если ответ и тогда обрезан, сервис вернёт ошибку вместо неполной фразы.

## Основные параметры

```text
--game-name                 Читаемое название игры
--parser                    Parser executable
--parser-arg                Аргумент parser; флаг можно повторять
--storage-dir               Каталог для .rag файлов
--embedding-url             Адрес embedding server
--embedding-model           Имя embedding-модели
--embedding-version         Версия embedding-модели
--embedding-dimensions      Размер embedding-вектора
--chunk-tokens              Максимальный размер chunk в токенах
--overlap-tokens            Перекрытие соседних chunks
--download-timeout          Таймаут загрузки страницы
--embedding-timeout         Таймаут запроса embeddings
```

Полный и актуальный список параметров:

```bash
./walkthrough-assistance knowledge add --help
```

Относительные пути из переменных окружения разрешаются относительно каталога
соответствующего бинарника, а не текущей директории запуска. Это относится к
`WALKTHROUGH_PARSER`, `WALKTHROUGH_PARSER_ARGS`,
`WALKTHROUGH_STORAGE_DIR`, `WALKTHROUGH_LLAMA_MODELS_PRESET` и
`WALKTHROUGH_SYSTEM_PROMPT_FILE`. Например, `./data/games` в `bin/.env`
означает `bin/data/games`. Абсолютные пути не изменяются; относительные значения
флагов CLI сохраняют стандартное поведение и разрешаются от текущей директории.
Для parser относительный путь должен содержать каталог, например `./parser.py`
или `../parser/python`; имя команды без `/`, например `python`, ищется в `PATH`.

## Проверка проекта

Запуск всех тестов:

```bash
go test ./...
```

Дополнительные проверки:

```bash
go test -race ./...
go vet ./...
```

## Текущие ограничения

- Из пользовательской функциональности реализованы ingestion через
  `knowledge add` и управление сервисом.
- Query, retrieval и генерация ответа через LLM пока недоступны из CLI.
- `/ask` использует vector retrieval по локальному `.rag`-файлу и llama.cpp.
- Автоматический поиск прохождений и регистрация игр пока не реализованы.

## Воспроизводимая оценка P0

[Evaluation](evaluation/README.md): автономный retrieval-only, локальное
сравнение `default`/`strict-ru`, oracle-context, manifest с GGUF SHA256 и trace
стадий. OpenCode не требуется. Для сервиса доступны
`WALKTHROUGH_PROMPT_PROFILE` и `WALKTHROUGH_RETRIEVAL_MODE=hybrid|vector`.
Гибридный поиск объединяет cosine и FTS5/BM25 через RRF; `cosine_score` в debug
не является итоговым hybrid-рангом. Отдельные оценки теперь доступны в trace.
