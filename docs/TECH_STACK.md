> Исторический проект архитектуры. Подготовка данных и создание `.rag` теперь находятся в отдельном модуле `data-preparation/`; актуальные команды и границы ответственности описаны в корневом README и `data-preparation/README.md`. Упоминания ingestion/Python parser внутри backend ниже не описывают текущую реализацию.

# Технический стек Walkthrough Assistance Backend

## Основной подход

Walkthrough Assistance Backend реализуется преимущественно на **Go**.

Go используется для всей резидентной части приложения:

* HTTP API;
* CLI;
* Game Resolver;
* Downloader;
* Chunker;
* Storage;
* FTS;
* Vector Search;
* Hybrid Retrieval;
* Spoiler filtering;
* Prompt Builder;
* LLM integration;
* Embeddings integration;
* Model Manager;
* управление `llama.cpp`;
* конфигурация;
* логирование.

Python используется только как отдельный вспомогательный инструмент для качественного извлечения основного содержимого walkthrough из HTML.

Python parser не является daemon и не работает постоянно. Он запускается Go backend только во время ingestion и завершается после преобразования HTML в нормализованный Markdown.

Основной runtime не должен зависеть от Python.

---

## Go

Требуемая версия:

```text
Go 1.26+
```

Основной backend собирается в один native executable:

```text
walkthrough-assistance
```

Целевая платформа MVP:

```text
Linux x86_64
SteamOS / Steam Deck
```

Желательно избегать CGO там, где это возможно.

---

## HTTP API

Использовать стандартную библиотеку Go:

```text
net/http
```

Дополнительный HTTP framework для MVP не требуется.

API должен слушать только:

```text
127.0.0.1
```

Пример:

```text
127.0.0.1:19273
```

Для маршрутизации использовать возможности `http.ServeMux` современных версий Go.

---

## Streaming

Для `/ask` использовать Server-Sent Events.

Реализация:

```text
net/http
http.ResponseController
Flush()
```

Формат событий:

```text
status
source
token
done
error
```

WebSocket для MVP не требуется.

---

## CLI

Использовать:

```text
github.com/spf13/cobra
```

CLI и HTTP API должны использовать один и тот же application/service layer.

Основные команды:

```text
walkthrough-assistance serve
walkthrough-assistance status
walkthrough-assistance games

walkthrough-assistance game add

walkthrough-assistance knowledge add <game-id> <url>
walkthrough-assistance knowledge status <game-id>

walkthrough-assistance search <game-id> "<query>"
walkthrough-assistance ask <game-id> "<question>"
```

CLI не должен быть реализован как клиент HTTP API собственного backend.

CLI должен иметь возможность напрямую использовать application services.

---

## Downloader

Downloader реализуется на Go.

Использовать:

```text
net/http
```

Downloader отвечает за:

* HTTP GET;
* redirects;
* timeout;
* User-Agent;
* Content-Type;
* сохранение исходного URL;
* базовую обработку HTTP ошибок;
* ограничение максимального размера скачиваемого документа.

Downloader не занимается очисткой или parsing HTML.

Результатом является:

```text
RawDocument
```

---

## HTML Parser / Normalizer

HTML extraction является единственным модулем MVP, реализованным на Python.

Основной инструмент:

```text
trafilatura
```

Допускаются fallback-механизмы на базе:

```text
readability-lxml
markdownify
html2text
```

Точная комбинация может корректироваться после тестирования на реальных walkthrough.

Задача parser:

```text
raw HTML
    ↓
article extraction
    ↓
remove boilerplate
    ↓
preserve document structure
    ↓
normalized Markdown
```

Необходимо максимально удалять:

* navigation;
* footer;
* advertising;
* related articles;
* cookie banners;
* website UI;
* повторяющийся boilerplate.

Необходимо сохранять:

* title;
* H1/H2/H3;
* paragraphs;
* lists;
* порядок секций.

---

## Интеграция Go ↔ Python Parser

Python parser должен быть отдельным helper executable или self-contained runtime bundle.

Не создавать отдельный Python HTTP service.

Не использовать FastAPI между Go и parser.

Взаимодействие:

```text
Go Backend
    │
    │ stdin / temporary file
    ▼
walkthrough-parser
    │
    │ stdout
    ▼
ParsedDocument
```

Предпочтительный формат результата:

```json
{
  "title": "Walkthrough title",
  "markdown": "# Chapter\n\n...",
  "metadata": {
    "language": "en"
  }
}
```

Go-код взаимодействует с parser через абстракцию:

```text
DocumentParser
```

Пример логической сигнатуры:

```text
Parse(RawDocument) -> ParsedDocument
```

Конкретная реализация MVP:

```text
TrafilaturaParser
```

Это позволяет позднее заменить Python parser или добавить game/site-specific parsers без изменения ingestion pipeline.

---

## Chunker

Chunker реализуется на Go.

Не использовать LangChain или другие RAG framework.

Использовать собственную deterministic логику.

Приоритет разделения:

```text
headings
↓
logical sections
↓
paragraphs
↓
token/size limit
```

Chunk должен сохранять metadata:

```text
game_id
document_id
title
section
sequence
text
```

Архитектура должна позволять позднее добавить:

```text
chapter
quest
location
spoiler_level
progress_marker
```

Chunk size и overlap задаются конфигурацией.

Начальные ориентиры:

```text
chunk size: 500–1200 tokens
overlap: 100–150 tokens
```

---

## SQLite

Основное локальное хранилище:

```text
SQLite
```

Отдельная vector database не используется.

Для доступа из Go использовать:

```text
modernc.org/sqlite
```

Предпочтение отдаётся pure-Go варианту без обязательного CGO.

Основные таблицы:

```text
games
game_identifiers
documents
sources
chunks
```

---

## Full-Text Search

Использовать:

```text
SQLite FTS5
```

FTS индекс создаётся как минимум по:

```text
title
section
text
```

FTS используется прежде всего для точных игровых терминов:

* NPC;
* quest names;
* item names;
* locations;
* bosses;
* уникальные игровые названия.

---

## Vector Search

Использовать:

```text
sqlite-vec
```

Предпочтительная Go-интеграция:

```text
modernc.org/sqlite/vec
```

Vectors должны храниться в той же SQLite базе, что и остальные данные приложения.

Не использовать для MVP:

```text
Qdrant
Chroma
Weaviate
Milvus
PostgreSQL + pgvector
```

---

## Hybrid Retrieval

Hybrid retrieval реализуется непосредственно на Go.

Pipeline:

```text
question
   │
   ├── FTS5
   │
   └── vector search
          │
          ▼
      hybrid merge
          │
          ▼
      spoiler filter
          │
          ▼
        top N
```

Для MVP использовать:

```text
Reciprocal Rank Fusion
```

или эквивалентный простой deterministic ranking.

Начальный вариант:

```text
FTS top 10
Vector top 10
↓
RRF
↓
top 6
```

Все значения должны задаваться конфигурацией.

Не использовать ML reranker в MVP.

---

## Embeddings

Embedding model запускается через:

```text
llama.cpp / llama-server
```

Backend обращается к нему по localhost HTTP.

Использовать OpenAI-compatible API llama.cpp.

Логическая абстракция:

```text
EmbeddingProvider
```

Методы:

```text
Embed(text)
EmbedBatch(texts)
```

Начальная модель:

```text
EmbeddingGemma-300M
```

Конкретная модель и quantization задаются конфигурацией.

Backend не должен быть связан с конкретной embedding-моделью.

---

## LLM

Генерация выполняется локально через:

```text
llama.cpp
llama-server
```

Backend общается с `llama-server` через localhost HTTP и OpenAI-compatible API.

Логическая абстракция:

```text
LLMProvider
```

Основные операции:

```text
Generate(request)
Stream(request)
```

Начальная модель:

```text
Qwen3.5-4B
```

Конкретный GGUF и quantization задаются конфигурацией.

Backend не должен зависеть от Qwen на уровне application architecture.

---

## llama.cpp Manager

Управление `llama-server` реализуется на Go.

Использовать:

```text
os/exec
```

Manager отвечает за:

* запуск процесса;
* остановку;
* healthcheck;
* graceful shutdown;
* определение состояния;
* передачу пути к модели;
* передачу runtime параметров;
* обработку аварийного завершения.

Архитектура должна поддерживать два режима:

```text
managed
external
```

### Managed

Backend самостоятельно запускает `llama-server`.

### External

Backend подключается к уже работающему серверу:

```text
http://127.0.0.1:8080
```

Для MVP достаточно полностью реализовать managed mode, но LLMProvider не должен зависеть от способа запуска сервера.

---

## Model Manager

Model Manager реализуется на Go.

Использовать:

```text
net/http
crypto/sha256
os
io
```

Он должен знать:

```text
model name
download URL
local path
checksum
version
```

Минимальные функции:

```text
Check()
Download()
Verify()
GetPath()
Status()
```

---

## HTTP client для llama.cpp

Использовать стандартный:

```text
net/http.Client
```

Отдельный OpenAI SDK не требуется.

Реализовать собственный небольшой client для используемых endpoint:

```text
/v1/chat/completions
/v1/embeddings
/health
```

Это позволяет контролировать:

* timeout;
* streaming;
* cancellation;
* ошибки;
* формат llama.cpp extensions.

---

## Configuration

Основной формат:

```text
YAML
```

Для parsing использовать:

```text
gopkg.in/yaml.v3
```

Конфигурация может переопределяться environment variables там, где это необходимо.

Пример:

```yaml
server:
  host: 127.0.0.1
  port: 19273

database:
  path: ~/.local/share/walkthrough-assistance/data/walkthrough.db

llm:
  provider: llama-cpp
  mode: managed
  model: qwen3.5-4b
  base_url: http://127.0.0.1:8080

embeddings:
  provider: llama-cpp
  model: embeddinggemma-300m
  base_url: http://127.0.0.1:8081

retrieval:
  fts_limit: 10
  vector_limit: 10
  final_limit: 6
  strategy: rrf
```

---

## Logging

Использовать стандартный structured logger:

```text
log/slog
```

Минимальные уровни:

```text
DEBUG
INFO
WARN
ERROR
```

Рекомендуемые structured fields:

```text
request_id
game_id
document_id
chunk_id
stage
duration_ms
fts_score
vector_score
final_score
```

В debug режиме логировать:

```text
FTS results
vector results
final ranking
selected chunks
LLM timings
ingestion stages
```

Полный пользовательский prompt и чувствительные пользовательские данные по умолчанию не логировать.

---

## Error Handling

На application level использовать typed/sentinel errors Go.

API должен преобразовывать их в стабильные error codes:

```text
GAME_NOT_RESOLVED
KNOWLEDGE_NOT_INSTALLED
KNOWLEDGE_INSTALLING
NO_RELEVANT_CONTEXT
LLM_UNAVAILABLE
EMBEDDING_MODEL_UNAVAILABLE
INGESTION_FAILED
PARSER_FAILED
```

HTTP handlers не должны самостоятельно определять business logic ошибок.

---

## Concurrency

Использовать стандартные механизмы Go:

```text
goroutines
channels
context.Context
sync
```

`context.Context` должен передаваться через основные application services.

Он используется для:

* cancellation HTTP request;
* отмены LLM generation;
* downloader timeout;
* graceful shutdown;
* ingestion cancellation.

Количество параллельных embedding/inference операций должно быть ограничено.

Не запускать несколько тяжёлых LLM inference одновременно без явной необходимости.

---

## Testing

Использовать стандартный:

```text
testing
```

Для HTTP:

```text
net/http/httptest
```

Основные уровни тестирования:

```text
unit tests
repository tests
retrieval tests
ingestion tests
API integration tests
```

Особенно важно иметь deterministic tests для:

```text
chunking
FTS search
RRF
game resolution
spoiler filtering
prompt construction
```

Для parser желательно сохранить небольшой fixture dataset HTML страниц walkthrough.

---

## Deployment

Основной backend:

```text
Go native binary
```

Daemon:

```text
systemd --user
```

Unit:

```text
walkthrough-assistance.service
```

Пример installation layout:

```text
~/.local/share/walkthrough-assistance/
├── bin/
│   ├── walkthrough-assistance
│   ├── walkthrough-parser
│   └── llama-server
│
├── parser/
│   └── runtime/
│
├── models/
│   ├── generator.gguf
│   └── embedding.gguf
│
├── data/
│   └── walkthrough.db
│
├── cache/
├── sources/
└── logs/
```

Установка не должна изменять read-only системную часть SteamOS.

---

## Компоненты, которые не использовать в MVP

Без отдельной необходимости не добавлять:

```text
FastAPI
Uvicorn
Gin
Fiber
Echo

LangChain
LlamaIndex

Qdrant
Chroma
Weaviate

Redis
PostgreSQL

Docker
Kubernetes

RabbitMQ
Kafka

gRPC
WebSocket

ORM
```

MVP должен оставаться максимально небольшим локальным приложением.

---

## Итоговый стек

```text
Primary language:
    Go 1.26+

Backend:
    net/http
    Cobra
    log/slog
    context
    os/exec

Downloader:
    Go net/http

Parser:
    Python
    Trafilatura
    readability-lxml fallback
    Markdown normalization

Storage:
    SQLite
    modernc.org/sqlite

Full-text search:
    SQLite FTS5

Vector search:
    sqlite-vec
    modernc.org/sqlite/vec

Chunking:
    custom Go implementation

Hybrid retrieval:
    custom Go implementation
    Reciprocal Rank Fusion

LLM:
    llama.cpp / llama-server
    OpenAI-compatible HTTP API

Embeddings:
    llama.cpp / llama-server
    EmbeddingGemma-300M initially

Generator:
    Qwen3.5-4B initially

Configuration:
    YAML
    gopkg.in/yaml.v3

Logging:
    log/slog

Process management:
    os/exec

Daemon:
    systemd --user

Target:
    SteamOS / Steam Deck
    Linux x86_64
```

## Архитектурный принцип

Runtime путь должен выглядеть так:

```text
Decky Plugin
      │
      │ HTTP + SSE
      ▼
Go Walkthrough Assistance Backend
      │
      ├── SQLite
      │    ├── FTS5
      │    └── sqlite-vec
      │
      └── llama-server
```

Python не участвует в обычной обработке пользовательского вопроса.

Python используется только в ingestion:

```text
URL
 │
 ▼
Go Downloader
 │
 ▼
Raw HTML
 │
 ▼
Python Trafilatura Parser
 │
 ▼
Normalized Markdown
 │
 ▼
Go Chunker
 │
 ▼
Embeddings
 │
 ▼
SQLite
```

Главная технологическая граница проекта:

```text
Go
=
вся application/runtime логика

Python
=
только качественное HTML content extraction

llama.cpp
=
локальный inference

SQLite
=
всё локальное knowledge storage и retrieval
```
