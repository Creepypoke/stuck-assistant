> Исторический проект архитектуры. Подготовка данных и создание `.rag` теперь находятся в отдельном модуле `data-preparation/`; актуальные команды и границы ответственности описаны в корневом README и `data-preparation/README.md`. Упоминания ingestion/Python parser внутри backend ниже не описывают текущую реализацию.

# Walkthrough Assistance Backend — MVP

## 1. Назначение проекта

Walkthrough Assistance Backend — локальный сервис для Steam Deck, который предоставляет интеллектуальный поиск по прохождениям игр и формирует ответы на вопросы пользователя с помощью локальной LLM.

Backend работает независимо от Decky-плагина и отвечает за:

* идентификацию текущей игры;
* загрузку и локальное хранение прохождений;
* преобразование прохождений в структурированную базу знаний;
* полнотекстовый и семантический поиск по прохождениям;
* формирование контекста для LLM;
* построение system/user prompt;
* генерацию ответа через локальную модель;
* предоставление API для Decky-плагина и других клиентов.

Основной сценарий:

1. Пользователь запускает игру на Steam Deck.
2. Decky-плагин определяет текущую игру.
3. Пользователь задаёт вопрос голосом или текстом.
4. Плагин передаёт вопрос и информацию об игре в Walkthrough Assistance Backend.
5. Backend находит релевантные фрагменты прохождения.
6. Backend формирует prompt.
7. Локальная LLM формирует краткий ответ.
8. Ответ возвращается в Decky-плагин.

После предварительной загрузки прохождения основной сценарий должен работать полностью offline.

---

# 2. Основные требования MVP

MVP должен поддерживать:

* Steam-игры;
* Non-Steam игры;
* несколько локально сохранённых игр;
* загрузку прохождения по URL или через provider;
* HTML → Markdown/text conversion;
* chunking;
* embeddings;
* локальное хранение;
* FTS-поиск;
* vector search;
* hybrid retrieval;
* локальную LLM через llama.cpp;
* генерацию ответа по найденным фрагментам;
* no-spoiler параметр на уровне API;
* HTTP API;
* streaming ответа;
* CLI для разработки и диагностики.

MVP не должен зависеть от Decky-плагина для своей работы.

Все основные сценарии должны быть доступны через CLI.

---

# 3. Не входит в MVP

На первом этапе не реализовывать:

* автоматический анализ save-файлов;
* автоматическое определение текущего квеста;
* полноценный vision-анализ screenshot;
* автоматическое определение игрового прогресса;
* сложный reranker;
* автоматический fallback между несколькими LLM;
* облачные LLM;
* автоматическое скачивание прохождений для всех установленных игр;
* синхронизацию между устройствами;
* multi-agent архитектуру.

Архитектура при этом должна позволять добавить эти возможности позднее.

---

# 4. Общая архитектура

```text
                    Internet
                       │
                       ▼
              Walkthrough Sources
                       │
                       ▼
                ┌──────────────┐
                │  Ingestion   │
                │              │
                │ download     │
                │ parse        │
                │ markdown     │
                │ chunk        │
                │ embedding    │
                └──────┬───────┘
                       │
                       ▼
                ┌──────────────┐
                │   Storage    │
                │              │
                │ SQLite       │
                │ FTS5         │
                │ vectors      │
                └──────┬───────┘
                       │
                       ▼
Decky ──IPC──► Query Engine
                       │
               ┌───────┴────────┐
               │                │
             FTS5          vector search
               │                │
               └───────┬────────┘
                       ▼
                 Hybrid Rank
                       │
                       ▼
                Spoiler Filter
                       │
                       ▼
                Prompt Builder
                       │
                       ▼
                  LLM Provider
                       │
                       ▼
                   llama.cpp
                       │
                       ▼
                    Answer
```

---

# 5. Структура проекта

Предлагаемая логическая структура:

```text
walkthrough-assistance-backend/

src/
├── api/
├── cli/
│
├── games/
│   ├── resolver/
│   ├── repository/
│   └── types/
│
├── ingestion/
│   ├── providers/
│   ├── downloader/
│   ├── parser/
│   ├── chunker/
│   └── service/
│
├── embeddings/
│   ├── provider/
│   └── service/
│
├── retrieval/
│   ├── fts/
│   ├── vector/
│   ├── hybrid/
│   └── service/
│
├── spoiler/
│
├── prompts/
│
├── llm/
│   ├── provider/
│   ├── llama-cpp/
│   └── service/
│
├── query/
│
├── storage/
│   ├── sqlite/
│   ├── migrations/
│   └── repositories/
│
├── models/
│   └── manager/
│
├── config/
│
└── application/
```

Физическая структура может отличаться, но границы модулей должны сохраняться.

---

# 6. Game Module

## Назначение

Game Module отвечает за каноническое представление игры внутри системы.

Нельзя использовать Steam AppID как основной внутренний идентификатор, поскольку Steam Deck поддерживает Non-Steam игры.

Backend должен использовать собственный:

```text
game_id
```

Например:

```text
witcher-3-wild-hunt
warcraft-3-reforged
tainted-grail-fall-of-avalon
```

Допускается UUID, но читаемый stable slug предпочтительнее для MVP.

## Модель игры

Пример:

```json
{
  "id": "witcher-3-wild-hunt",
  "title": "The Witcher 3: Wild Hunt",
  "releaseYear": 2015
}
```

Внешние идентификаторы хранятся отдельно.

Пример:

```json
[
  {
    "type": "steam_app_id",
    "value": "292030"
  },
  {
    "type": "executable",
    "value": "witcher3.exe"
  }
]
```

---

# 7. Game Resolver

## Назначение

Game Resolver преобразует информацию о запущенной игре в канонический `game_id`.

На вход приходит `LaunchContext`.

Steam-вариант:

```json
{
  "source": "steam",
  "steamAppId": 292030,
  "displayName": "The Witcher 3"
}
```

Non-Steam:

```json
{
  "source": "non-steam",
  "shortcutId": "123456789",
  "displayName": "Warcraft III",
  "executable": "Warcraft III.exe"
}
```

Результат:

```json
{
  "gameId": "warcraft-3-reforged",
  "confidence": 1.0
}
```

## Поведение MVP

Resolution выполняется в следующем порядке:

1. поиск точного mapping по Steam AppID;
2. поиск по Non-Steam shortcut ID;
3. поиск по executable;
4. поиск по сохранённому manual mapping;
5. если соответствие не найдено — вернуть `unresolved`.

Автоматическое fuzzy-определение игры не обязательно для MVP.

Backend должен предоставлять API/CLI для ручного mapping.

---

# 8. Ingestion Module

## Назначение

Отвечает за превращение интернет-прохождения в локальную базу знаний.

Pipeline:

```text
Source
 ↓
Download
 ↓
Parse
 ↓
Normalize
 ↓
Markdown
 ↓
Chunk
 ↓
Embed
 ↓
Store
```

---

# 9. Walkthrough Providers

Provider — адаптер источника прохождений.

Интерфейс должен примерно соответствовать:

```text
WalkthroughProvider

find(game)
download(source)
```

В MVP достаточно:

* Generic URL Provider;
* одного специализированного provider при необходимости.

Generic provider получает URL вручную и скачивает страницу.

Пример CLI:

```bash
walkthrough-assistance knowledge add \
  witcher-3-wild-hunt \
  https://example.com/walkthrough
```

Автоматический поиск лучших прохождений можно добавить позже.

---

# 10. Downloader

Отвечает только за получение исходного документа.

Должен поддерживать:

* HTTP GET;
* redirect;
* timeout;
* User-Agent;
* сохранение исходного URL;
* базовую обработку ошибок.

Downloader не занимается parsing.

Результат:

```text
RawDocument
```

с полями:

```json
{
  "url": "...",
  "contentType": "text/html",
  "content": "...",
  "downloadedAt": "..."
}
```

---

# 11. Parser / Normalizer

## Назначение

Преобразует HTML в чистый текст/Markdown.

Необходимо удалить:

* навигацию;
* footer;
* рекламу;
* related articles;
* cookie banners;
* лишние ссылки;
* UI сайта.

Сохранить по возможности:

* H1/H2/H3;
* параграфы;
* списки;
* последовательность секций.

Результат:

```markdown
# Main Quest

## Entering the Castle

...

## Finding the Key

...
```

Исходный HTML может сохраняться отдельно для повторной обработки.

---

# 12. Chunker

## Назначение

Разбивает walkthrough на небольшие смысловые фрагменты для retrieval.

Приоритет разбиения:

1. headings;
2. логические секции;
3. параграфы;
4. token limit.

Не использовать тупое фиксированное разбиение всего документа, если доступна структура.

Целевой размер одного chunk:

```text
примерно 500–1200 tokens
```

Допускается overlap:

```text
100–150 tokens
```

Chunk должен содержать metadata.

Пример:

```json
{
  "gameId": "witcher-3-wild-hunt",
  "documentId": "doc-123",
  "title": "Bloody Baron",
  "section": "Investigate the Room",
  "sequence": 42,
  "text": "..."
}
```

Дополнительные metadata могут быть добавлены позже:

```text
chapter
quest
location
spoiler_level
progress_marker
```

---

# 13. Embeddings Module

## Назначение

Создаёт vector representation chunk'ов и пользовательских запросов.

Embedding provider должен быть абстрагирован.

Например:

```text
EmbeddingProvider

embed(text)
embedBatch(texts)
```

В MVP используется локальная embedding-модель через llama.cpp либо отдельный llama.cpp server.

Рекомендуемый начальный вариант:

```text
EmbeddingGemma-300M
```

но конкретная модель должна задаваться конфигурацией.

---

# 14. Storage Module

Для MVP использовать SQLite.

Не поднимать отдельную vector database.

Основные сущности:

```text
games
game_identifiers
documents
chunks
sources
```

Минимальная схема:

```text
games
-----
id
title
release_year

game_identifiers
----------------
id
game_id
type
value

documents
---------
id
game_id
source_url
title
created_at

chunks
------
id
document_id
game_id
title
section
sequence
text
embedding
```

FTS5 индекс должен быть создан по:

```text
title
section
text
```

Vector storage может использовать `sqlite-vec` либо аналогичное SQLite extension.

---

# 15. Retrieval Module

## Назначение

Находит контекст, необходимый для ответа на вопрос.

Pipeline:

```text
question
   │
   ├──── FTS5
   │
   └──── Vector Search
          │
          ▼
     Hybrid Merge
          │
          ▼
      Filtering
          │
          ▼
        top N
```

Retrieval всегда должен ограничиваться текущей игрой:

```text
game_id = current_game
```

---

# 16. FTS Retrieval

Используется SQLite FTS5.

Особенно полезен для:

* имён персонажей;
* названий квестов;
* предметов;
* локаций;
* боссов;
* уникальных терминов.

Возвращает:

```text
chunk_id
score
```

---

# 17. Vector Retrieval

Query преобразуется embedding-моделью в vector.

Выполняется поиск ближайших chunks только для текущего `game_id`.

Возвращает:

```text
chunk_id
similarity
```

---

# 18. Hybrid Retrieval

Результаты FTS и semantic search объединяются.

На первом этапе не требуется сложный ML-ranking.

Допускается:

* normalization scores;
* weighted sum;
* Reciprocal Rank Fusion.

Например:

```text
FTS top 10
+
Vector top 10
↓
RRF
↓
top 6
```

Значения должны задаваться конфигурацией.

---

# 19. Spoiler Module

## Назначение

Контролирует передачу потенциально будущей информации в LLM.

API принимает:

```json
{
  "noSpoilers": true
}
```

В MVP полноценное автоматическое определение прогресса отсутствует.

Поэтому no-spoiler режим MVP состоит из двух уровней:

### Retrieval level

Если chunk содержит progress metadata, исключать явно будущие chunks.

### Prompt level

Добавлять правило:

```text
Do not reveal future plot events or consequences
unless they are necessary to answer the current question.
```

Архитектура должна позволять позднее передавать:

```json
{
  "chapter": "...",
  "quest": "...",
  "progress": "..."
}
```

и выполнять полноценную фильтрацию.

---

# 20. Prompt Builder

## Назначение

Только backend отвечает за prompt construction.

Decky-плагин не должен передавать system prompt.

На вход Prompt Builder получает:

```text
game
question
retrieved chunks
player context
options
```

Результат:

```text
LLM request
```

Пример:

```text
SYSTEM:

You are an in-game walkthrough assistant.

Answer using the provided walkthrough excerpts.

Do not invent missing information.

If the walkthrough does not contain enough information,
say that you could not find the answer.

Avoid unnecessary spoilers.

GAME:
The Witcher 3: Wild Hunt

CONTEXT:

[1]
Title: Bloody Baron
Section: Investigate the Room
...

[2]
...

QUESTION:

Where can I find the missing item?
```

Prompt templates должны храниться отдельно от Query Engine.

---

# 21. LLM Module

## Назначение

Абстрагирует Query Engine от конкретной модели.

Интерфейс:

```text
LLMProvider

generate(request)
stream(request)
```

Для MVP:

```text
LlamaCppProvider
```

работает через OpenAI-compatible API llama.cpp.

Начальная модель:

```text
Qwen3.5-4B
```

Точный GGUF и quantization задаются конфигурацией.

Backend не должен предполагать, что Qwen будет использоваться всегда.

В будущем возможны:

```text
OpenAIProvider
CloudProvider
LlamaCppProvider
```

без изменений внешнего API.

---

# 22. Query Engine

## Назначение

Главный orchestration layer runtime-части.

Алгоритм:

```text
ask(request)

1. Resolve game
2. Validate knowledge availability
3. Retrieve FTS results
4. Retrieve vector results
5. Merge/rank
6. Apply spoiler filtering
7. Select context
8. Build prompt
9. Call LLM
10. Stream response
11. Return sources
```

Query Engine не должен самостоятельно:

* читать SQLite напрямую;
* вызывать llama.cpp напрямую;
* скачивать walkthrough;
* строить embeddings.

Для этого используются соответствующие сервисы.

---

# 23. API

MVP использует localhost HTTP.

Слушать только:

```text
127.0.0.1
```

Пример порта:

```text
19273
```

## Health

```http
GET /health
```

Ответ:

```json
{
  "status": "ok",
  "llm": "ready",
  "embedding": "ready",
  "database": "ready"
}
```

---

## Games

```http
GET /games
```

Возвращает зарегистрированные игры.

---

## Resolve game

```http
POST /games/resolve
```

Request:

```json
{
  "source": "non-steam",
  "shortcutId": "...",
  "displayName": "Warcraft III",
  "executable": "Warcraft III.exe"
}
```

---

## Manual mapping

```http
POST /games/mappings
```

Позволяет связать неизвестный LaunchContext с `game_id`.

---

## Knowledge status

```http
GET /games/{gameId}/knowledge
```

Пример:

```json
{
  "status": "ready",
  "documents": 4,
  "chunks": 821
}
```

---

## Add walkthrough

```http
POST /games/{gameId}/knowledge/sources
```

```json
{
  "url": "https://..."
}
```

Запускает ingestion.

---

## Ask

```http
POST /ask
```

Request:

```json
{
  "game": {
    "source": "steam",
    "steamAppId": 292030,
    "displayName": "The Witcher 3"
  },
  "question": "Where do I find the key?",
  "context": {
    "location": null,
    "quest": null,
    "progress": null
  },
  "options": {
    "noSpoilers": true,
    "answerLength": "short"
  }
}
```

---

# 24. Streaming

`/ask` должен поддерживать streaming.

Для MVP использовать SSE.

Пример событий:

```text
event: status
data: {"stage":"retrieval"}

event: source
data: {"title":"Bloody Baron","section":"Investigate the Room"}

event: status
data: {"stage":"generation"}

event: token
data: {"text":"The key"}

event: token
data: {"text":" can be found..."}

event: done
data: {}
```

---

# 25. Sources

Ответ должен содержать информацию о том, какие chunks были использованы.

Например:

```json
{
  "source": {
    "documentTitle": "Main Walkthrough",
    "section": "Entering the Castle",
    "url": "https://..."
  }
}
```

Это необходимо как для UI, так и для debugging качества RAG.

---

# 26. CLI

Backend должен быть полноценно тестируемым без Decky.

Команды MVP:

```bash
walkthrough-assistance status
```

```bash
walkthrough-assistance games
```

```bash
walkthrough-assistance game add
```

```bash
walkthrough-assistance knowledge add <game-id> <url>
```

```bash
walkthrough-assistance knowledge status <game-id>
```

```bash
walkthrough-assistance ask <game-id> "question"
```

```bash
walkthrough-assistance serve
```

Опционально:

```bash
walkthrough-assistance search <game-id> "query"
```

Эта команда особенно полезна для отладки retrieval без LLM.

---

# 27. Model Manager

## Назначение

Управляет файлами локальных моделей.

Должен знать:

```text
model name
download URL
local path
checksum
version
```

Для MVP достаточно:

* проверить наличие модели;
* скачать отсутствующую;
* показать статус;
* предоставить путь llama.cpp.

Не требуется полноценный UI model manager.

---

# 28. llama.cpp Manager

Backend должен уметь работать с llama.cpp как с внешним процессом.

MVP допускает два режима:

```text
managed
external
```

### Managed

Backend самостоятельно запускает:

```text
llama-server
```

### External

Адрес сервера задаётся через config:

```text
http://127.0.0.1:8080
```

В MVP достаточно реализовать один режим, но код не должен жёстко зависеть от конкретного способа запуска.

---

# 29. Конфигурация

Пример:

```yaml
server:
  host: 127.0.0.1
  port: 19273

database:
  path: ~/.local/share/walkthrough-assistance/data.db

llm:
  provider: llama-cpp
  base_url: http://127.0.0.1:8080
  model: qwen3.5-4b

embeddings:
  provider: llama-cpp
  base_url: http://127.0.0.1:8081

retrieval:
  fts_limit: 10
  vector_limit: 10
  final_limit: 6
```

---

# 30. Файловая структура установки

Все данные хранить в пользовательском home.

Например:

```text
~/.local/share/walkthrough-assistance/
├── bin/
│   ├── walkthrough-assistance
│   └── llama-server
│
├── models/
│   ├── generator.gguf
│   └── embedding.gguf
│
├── data/
│   └── walkthrough.db
│
├── cache/
│
├── sources/
│
└── logs/
```

Не требовать изменения read-only системной части SteamOS.

---

# 31. Запуск

Backend должен работать как пользовательский сервис.

Предпочтительно:

```text
systemd --user
```

Сервис:

```text
walkthrough-assistance.service
```

`.desktop` используется отдельно для:

* установки;
* обновления;
* запуска setup/configuration UI;
* диагностики.

`.desktop` не должен быть основным способом поддерживать daemon в запущенном состоянии.

---

# 32. Логирование

Минимально логировать:

* startup;
* состояние моделей;
* запросы без пользовательских чувствительных данных;
* retrieval scores;
* выбранные chunks;
* ingestion;
* ошибки HTTP;
* ошибки llama.cpp.

Debug mode должен позволять увидеть:

```text
FTS results
Vector results
Final ranking
Prompt context
LLM timings
```

Это критично для дальнейшей настройки RAG.

---

# 33. Ошибки API

Backend должен явно различать ситуации:

```text
GAME_NOT_RESOLVED

KNOWLEDGE_NOT_INSTALLED

KNOWLEDGE_INSTALLING

NO_RELEVANT_CONTEXT

LLM_UNAVAILABLE

EMBEDDING_MODEL_UNAVAILABLE

INGESTION_FAILED
```

Decky должен иметь возможность показать пользователю понятное состояние.

---

# 34. Производительность

Система проектируется под Steam Deck.

Приоритеты:

1. небольшое потребление RAM;
2. минимальное количество фоновых процессов;
3. отсутствие тяжёлой standalone vector DB;
4. быстрый retrieval;
5. возможность полностью выгружать LLM при необходимости.

FTS/vector retrieval должен занимать существенно меньше времени, чем генерация ответа.

---

# 35. Будущие расширения

Архитектура должна позволять добавить без серьёзного рефакторинга:

### Confidence fallback

```text
small LLM
   ↓
confidence
   ↓
large LLM if necessary
```

### Reranker

```text
hybrid retrieval
      ↓
top 20
      ↓
reranker
      ↓
top 5
```

### Cloud LLM

```text
LLMProvider
├── llama.cpp
└── OpenAI
```

### Game progress providers

```text
ProgressProvider
├── Manual
├── SaveFile
├── Screenshot
└── Game-specific adapter
```

### Vision

Передача screenshot и определение:

* текущей локации;
* NPC;
* квеста;
* объекта;
* UI-состояния.

### Automatic walkthrough discovery

Автоматический поиск подходящих walkthrough для установленной игры.

---

# 36. MVP End-to-End сценарий

После реализации должна работать следующая последовательность.

Пользователь регистрирует игру:

```bash
walkthrough-assistance game add \
  "Tainted Grail: The Fall of Avalon"
```

Добавляет walkthrough:

```bash
walkthrough-assistance knowledge add \
  tainted-grail-fall-of-avalon \
  https://example.com/walkthrough
```

Backend:

```text
downloads page
↓
extracts article
↓
converts to Markdown
↓
splits into chunks
↓
generates embeddings
↓
stores everything in SQLite
↓
builds FTS index
```

Пользователь отключает интернет.

Затем выполняет:

```bash
walkthrough-assistance ask \
  tainted-grail-fall-of-avalon \
  "How do I open the gate?"
```

Backend:

```text
FTS search
+
vector search
↓
hybrid rank
↓
top chunks
↓
prompt
↓
Qwen3.5-4B
↓
answer
```

Ответ формируется полностью offline.

Тот же запрос должен быть доступен через HTTP API для Decky-плагина.

---

# 37. Критерии готовности MVP

MVP считается готовым, если выполняются следующие условия.

1. Backend запускается на Steam Deck независимо от Decky.

2. Можно создать или зарегистрировать Steam и Non-Steam игру.

3. Можно вручную добавить URL прохождения.

4. Backend успешно скачивает и очищает страницу.

5. Прохождение разбивается на chunks.

6. Для chunks создаются embeddings.

7. Всё сохраняется локально в SQLite.

8. Можно выполнить FTS search без LLM.

9. Можно выполнить vector search без LLM.

10. Работает hybrid retrieval.

11. Query Engine передаёт только выбранные chunks в LLM.

12. Qwen3.5-4B генерирует ответ через llama.cpp.

13. После ingestion запрос работает без подключения к интернету.

14. API `/ask` доступен локальному Decky-плагину.

15. Ответ `/ask` может стримиться.

16. В ответе доступны использованные источники.

17. System prompt полностью формируется внутри backend.

18. Steam AppID не является внутренним primary key игры.

19. Non-Steam игры могут быть связаны с внутренним `game_id`.

20. Backend можно использовать и отлаживать через CLI без Decky.

---

# 38. Конечный ожидаемый результат

Результатом MVP является устанавливаемый на Steam Deck локальный сервис:

```text
Walkthrough Assistance Backend
```

который предоставляет универсальный API:

```text
game + question + context
             ↓
          backend
             ↓
          answer
```

и скрывает от клиента всю внутреннюю реализацию:

```text
walkthrough sources
HTML parsing
Markdown
chunking
embeddings
SQLite
FTS
vector search
hybrid retrieval
spoiler filtering
prompt construction
llama.cpp
Qwen
```

Decky-плагин в конечной архитектуре должен воспринимать Walkthrough Assistance Backend как самостоятельный локальный сервис и не зависеть от того, какие модели, embeddings, источники или retrieval-алгоритмы используются внутри.

Главный контракт проекта:

```text
Decky Plugin
=
UI + voice + game integration

Walkthrough Assistance Backend
=
game knowledge + RAG + prompt construction + inference
```

MVP должен доказать основной сценарий:

> для заранее загруженной игры пользователь может без интернета задать естественный вопрос по прохождению и получить короткий релевантный ответ от локальной модели.
