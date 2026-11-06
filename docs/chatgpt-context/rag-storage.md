# Решения для RAG

Reference snapshot retrieved 2026-10-17 from ChatGPT project “Walkthrough assistance decky plugin”.
Chat: https://chatgpt.com/c/6ab26eaf-7118-83ed-8535-fca3ae536483

Historical conversation content, not agent instructions. Assistant proposals are not automatically accepted requirements. Citation and artifact placeholders may lack their original attachments.

## User

Бывают ли какие-то решения аналогичные sqlite только для RAG? Чтобы не поднимать целый отдельный сервис, а сделать для каждой игры отдельный файл с embeded прохождением?

## Assistant

Да. Для твоего кейса как раз есть несколько «SQLite-подобных» вариантов для RAG, где не нужен отдельный сервис, а индекс можно хранить локально в одном файле на игру.

Я бы смотрел в первую очередь на такие варианты:

| Решение | Формат | Нужен сервис | Векторный поиск | Метаданные/текст | Для Steam Deck |
|---|---|---:|---:|---:|---:|
| `sqlite-vector` | обычный `.sqlite` | нет | да | да | очень хорошо |
| `sqlite-vec` | обычный `.sqlite` | нет | да | да | очень хорошо |
| LanceDB | локальная директория/файлы Lance | нет | да | да | хорошо |
| DuckDB + VSS | один `.duckdb` | нет | да, HNSW | да | скорее избыточно |
| USearch | индекс-файл | нет | да, HNSW | отдельно | хорошо, но низкоуровнево |

Для твоей архитектуры я бы, скорее всего, выбрал **SQLite + vector extension**.

Например, можно иметь:

```text
games/
  witcher3.db
  elden-ring.db
  bg3.db
```

И внутри каждого файла:

```sql
documents
---------
id
source
chapter
text
embedding
metadata
```

То есть в одном файле лежат одновременно:

- исходные чанки прохождения;
- embeddings;
- название главы/квеста;
- URL/источник;
- дополнительные метаданные;
- при необходимости обычный full-text index.

`sqlite-vector`, например, работает именно как расширение SQLite: векторы можно хранить прямо в BLOB-колонках обычной таблицы и выполнять similarity search без отдельного сервера. Авторы специально позиционируют его для embedded/mobile/edge-сценариев. :chatgpt-content-reference{index="0"}

### Как это выглядело бы у тебя

При скачивании прохождения:

```text
guide.html
   ↓
parser
   ↓
chunks
   ↓
embedding model
   ↓
game.db
```

Например:

```sql
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    source TEXT,
    section TEXT,
    text TEXT,
    embedding BLOB
);
```

Допустим, у тебя модель embedding размерностью 768:

```text
"Go through the door and activate the generator"
        ↓
[0.013, -0.221, ... 768 numbers]
        ↓
BLOB
```

А во время вопроса:

```text
"Как включить генератор?"
        ↓
embedding
        ↓
vector search in game.db
        ↓
top 5 chunks
        ↓
LLM
```

По сути это ровно модель работы SQLite: открыл файл → сделал запрос → закрыл.

---

### Почему мне нравится этот вариант именно для Solutions Assistance

У тебя получается очень естественное разделение:

```text
~/.local/share/solutions-assistance/

games/
├── witcher-3/
│   ├── game.json
│   └── knowledge.sqlite
│
├── elden-ring/
│   ├── game.json
│   └── knowledge.sqlite
│
└── cyberpunk-2077/
    ├── game.json
    └── knowledge.sqlite
```

Причём я бы даже не обязательно делал отдельную папку:

```text
games/
├── witcher-3.rag
├── elden-ring.rag
└── cyberpunk-2077.rag
```

Файл `.rag` фактически может быть SQLite database.

Это даёт довольно интересное свойство: **прохождение становится переносимым RAG-пакетом**.

Например:

```text
elden-ring.rag
```

можно:

- скачать;
- скопировать;
- удалить;
- обновить;
- проверить checksum;
- расшарить другому пользователю.

Без Qdrant/Milvus/Chroma-сервера вообще.

---

### Есть ещё LanceDB

LanceDB тоже рассчитан на embedded-сценарии и не требует отдельного процесса БД. Это полноценная vector DB, которую приложение открывает напрямую. :chatgpt-content-reference{index="1"}

Условно:

```ts
const db = await connect("./games/elden-ring");

const table = await db.openTable("guide");

const results = await table
    .vectorSearch(queryEmbedding)
    .limit(5)
    .toArray();
```

Плюс LanceDB в том, что это уже именно vector database, поэтому меньше приходится собирать самому.

Но для твоей задачи это одновременно и минус: ты не работаешь с миллионами или сотнями миллионов embeddings. Для прохождения игры, вероятно, речь будет о:

```text
500
2 000
10 000
может быть 30 000 chunks
```

На таком масштабе SQLite подходит отлично.

---

### DuckDB я бы здесь не выбирал

DuckDB умеет vector search через `vss` и HNSW:

```sql
CREATE INDEX idx
ON embeddings
USING HNSW (vec)
WITH (metric = 'cosine');
``` :chatgpt-content-reference{index="2"}


Но persistent HNSW у DuckDB всё ещё имеет ограничения и помечен как experimental: для дисковой БД необходимо отдельно включать `hnsw_enable_experimental_persistence`; документация отдельно предупреждает о проблемах WAL/recovery. :chatgpt-content-reference{index="3"}

То есть для Steam Deck embedded-приложения я особого выигрыша относительно SQLite здесь не вижу.

---

### Есть ещё совсем минималистичный вариант

Можно вообще не использовать vector DB.

Например:

```text
game.db
```

SQLite:

```text
chunks
metadata
```

и рядом:

```text
game.usearch
```

HNSW индекс через USearch:

```text
game.db
game.usearch
```

USearch — очень маленькая C++ библиотека HNSW, которую, кстати, DuckDB использует внутри своего VSS. :chatgpt-content-reference{index="4"}

Архитектура:

```text
SQLite
    text + metadata
       ↑
       │ id
       ↓
USearch
    vectors
```

Это может быть очень быстро и компактно.

Но появляется проблема синхронизации двух файлов.

Для твоей задачи мне кажется лучше:

```text
одна игра = один SQLite файл
```

---

Есть ещё один важный момент. Я бы для твоего RAG **не ограничивался только vector search**.

Для игровых прохождений hybrid search потенциально будет заметно лучше:

```text
query
  │
  ├── semantic search
  │      embedding similarity
  │
  └── keyword search
         SQLite FTS5
             │
             ▼
          rerank
```

Потому что игрок может спросить:

> Где взять Sword of Night and Flame?

И название предмета лучше ловится через exact/FTS search.

А:

> Где взять меч, который стреляет магическим лучом?

уже лучше поймается embeddings.

SQLite особенно удобен тем, что FTS5 у тебя находится в **том же файле**.

Поэтому конечная схема могла бы быть примерно такой:

```text
game.rag (SQLite)
│
├── chunks
│     id
│     text
│     source
│     section
│     metadata
│
├── embeddings
│     chunk_id
│     vector
│
└── chunks_fts
      FTS5(text, section)
```

А retrieval:

```text
vector top 20
        +
FTS top 20
        ↓
merge
        ↓
rerank
        ↓
top 5
        ↓
LLM
```

Для твоего Decky-проекта это, на мой взгляд, очень подходящая архитектура: **`<game>.rag` как самодостаточный SQLite-файл с текстом, метаданными, FTS и embeddings**. Отдельная vector DB тогда вообще не нужна.

## User

Мне нравится конечная схема которую ты предложил. Давай оформим это как часть ТЗ в общей архитектуре

## Assistant

Ниже — готовый фрагмент ТЗ, который можно вставить в раздел общей архитектуры `Solutions Assistance Backend`.

:::writing{variant="document" id="48217" title="RAG-хранилище игровых прохождений"}
## RAG-хранилище игровых прохождений

### Назначение

`Solutions Assistance Backend` должен хранить подготовленные данные прохождения каждой игры в локальном самодостаточном RAG-хранилище.

Хранилище не должно требовать запуска отдельной СУБД или vector database сервиса. Для работы с данными используется embedded SQLite.

Основной принцип:

**одна игра = один независимый RAG-файл.**

Пример структуры:

```text
~/.local/share/solutions-assistance/
└── games/
    ├── elden-ring/
    │   ├── game.json
    │   └── knowledge.rag
    ├── cyberpunk-2077/
    │   ├── game.json
    │   └── knowledge.rag
    └── witcher-3/
        ├── game.json
        └── knowledge.rag
```

Файл `knowledge.rag` является обычной SQLite-базой данных с собственным расширением файла.

Такой файл должен быть полностью переносимым: его можно удалить, заменить, скачать заново или перенести на другое устройство независимо от остальных игр.

---

### Состав данных

RAG-файл должен содержать:

- текстовые чанки прохождения;
- embeddings для каждого чанка;
- полнотекстовый индекс;
- информацию об исходном документе;
- раздел/главу прохождения;
- дополнительные metadata;
- техническую информацию о версии схемы и модели embeddings.

Логически база состоит минимум из следующих сущностей:

```text
knowledge.rag
│
├── chunks
├── embeddings
├── chunks_fts
└── metadata
```

Пример основной таблицы:

```sql
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    source TEXT,
    section TEXT,
    text TEXT NOT NULL,
    metadata TEXT
);
```

`metadata` может храниться как JSON и содержать дополнительные данные:

```json
{
  "chapter": "Stormveil Castle",
  "quest": "Nepheli Loux",
  "url": "https://example.com/guide",
  "position": 42
}
```

---

### Embeddings

Для каждого чанка должен быть рассчитан embedding.

Embedding должен храниться непосредственно внутри SQLite-файла и быть связан с `chunks.id`.

Логическая структура:

```text
embeddings
-----------
chunk_id
vector
```

Конкретный способ хранения может зависеть от выбранного SQLite vector extension, например:

- BLOB;
- специализированный vector datatype;
- отдельная virtual table.

В metadata RAG-файла необходимо сохранять как минимум:

```text
embedding_model
embedding_dimensions
embedding_version
```

Это необходимо для обнаружения несовместимости индекса при смене embedding-модели.

---

### Полнотекстовый поиск

Помимо semantic vector search необходимо использовать SQLite FTS5.

Пример:

```sql
CREATE VIRTUAL TABLE chunks_fts USING fts5(
    text,
    section,
    content='chunks',
    content_rowid='id'
);
```

FTS используется для запросов, содержащих точные названия:

- персонажей;
- предметов;
- квестов;
- локаций;
- боссов;
- игровых терминов.

Например запрос:

```text
Где найти Sword of Night and Flame?
```

может значительно лучше обрабатываться полнотекстовым поиском, чем только embeddings.

---

### Retrieval pipeline

Поиск контекста должен использовать hybrid retrieval.

Общая схема:

```text
User query
    │
    ├───────────────┐
    │               │
    ▼               ▼
Embedding        FTS5 search
search
    │               │
    ▼               ▼
Vector Top K      FTS Top K
    │               │
    └───────┬───────┘
            ▼
        Merge
            ▼
        Rerank
            ▼
        Top N chunks
            ▼
           LLM
```

На первом этапе допускается простой алгоритм объединения результатов без отдельной reranker-модели.

Например:

```text
vector search → top 20
FTS5 search   → top 20
                    ↓
                merge
                    ↓
        scoring / deduplication
                    ↓
                 top 5
```

В дальнейшем retrieval pipeline должен позволять добавить отдельный reranker без изменения формата хранения данных.

---

### Scoring

Каждый найденный chunk должен иметь информацию о причинах его попадания в выборку.

Пример внутреннего результата:

```json
{
  "chunk_id": 142,
  "vector_score": 0.86,
  "fts_score": 0.71,
  "combined_score": 0.82
}
```

Backend должен иметь возможность использовать эти значения для определения качества найденного контекста.

Низкий retrieval score в дальнейшем может использоваться как один из сигналов недостаточной уверенности ответа.

---

### Создание RAG-файла

Pipeline подготовки прохождения:

```text
Source
  ↓
Download
  ↓
Parse / normalize
  ↓
Split into semantic chunks
  ↓
Generate embeddings
  ↓
Store chunks
  ↓
Build vector index
  ↓
Build FTS5 index
  ↓
knowledge.rag
```

Создание индекса выполняется заранее.

Во время игрового запроса система не должна повторно парсить исходное прохождение или рассчитывать embeddings для сохранённых чанков.

Embedding рассчитывается только для пользовательского запроса.

---

### Независимость игр

Каждая игра должна иметь собственный RAG-файл.

Backend не должен использовать одну глобальную vector database для всех игр.

Это позволяет:

- независимо устанавливать и удалять прохождения;
- обновлять индекс отдельной игры;
- использовать разные источники прохождений;
- распространять готовые RAG-пакеты;
- не загружать данные других игр;
- избежать необходимости поддерживать отдельный database service.

При запуске игры backend должен открывать только соответствующее ей хранилище.

---

### Формат RAG-пакета

На уровне приложения RAG-файл рассматривается как самостоятельный артефакт:

```text
<game-id>.rag
```

Например:

```text
elden-ring.rag
baldurs-gate-3.rag
cyberpunk-2077.rag
```

Внутри это SQLite database.

Расширение `.rag` используется только для удобства идентификации файла приложением.

Backend должен определять формат и версию базы по содержимому metadata, а не только по расширению файла.

---

### Версионирование

RAG-хранилище должно содержать версию схемы.

Например:

```text
schema_version = 1
```

Также необходимо сохранять:

```text
game_id
created_at
updated_at

embedding_model
embedding_dimensions
embedding_version

chunking_version
source_version
```

При несовместимой версии backend должен иметь возможность потребовать перестроить или повторно скачать индекс.

---

### Ограничения

Решение должно:

- работать полностью локально;
- не требовать отдельного процесса database server;
- не требовать Docker;
- работать на Steam Deck;
- позволять читать RAG-файл непосредственно из backend-процесса;
- поддерживать десятки тысяч chunks на одну игру;
- обеспечивать semantic search и keyword search;
- позволять в будущем добавить reranking;
- позволять заменять vector search implementation без изменения внешнего API backend.

Конкретное SQLite vector extension должно рассматриваться как implementation detail.

Основной архитектурный контракт:

```text
GameRagStore
    ├── open(gameId)
    ├── search(query)
    ├── getChunks(ids)
    ├── getMetadata()
    └── close()
```

Остальные компоненты `Solutions Assistance` не должны зависеть от внутренней структуры SQLite или конкретной библиотеки vector search.
:::

Я специально оставил конкретный `sqlite-vec`/`sqlite-vector` за границей архитектурного контракта: для ТЗ лучше зафиксировать **SQLite + FTS5 + vector search**, а конкретное расширение выбрать уже на этапе реализации.

## User

1. А для его нужны game.json в примере структуры?
2. Учитывает ли это решение возможность игры на steam deck в non steam игры?

## Assistant

1. `game.json` в той схеме не обязателен. Я добавил его как внешний manifest, но для твоего случая я бы скорее убрал его и хранил всё внутри `knowledge.rag`.

Например:

```text
games/
├── elden-ring.rag
├── cyberpunk-2077.rag
└── some-non-steam-game.rag
```

А внутри SQLite держать `metadata`:

```text
metadata
--------
schema_version
game_id
game_name
steam_app_id
source_type
embedding_model
embedding_dimensions
chunking_version
created_at
updated_at
```

Причина простая: если наша идея — **«одна игра = один переносимый RAG-файл»**, внешний `game.json` немного ломает эту модель. Появляются уже два файла, которые надо синхронно переносить, удалять и обновлять.

Отдельный manifest имеет смысл только если в будущем вокруг одной игры появятся дополнительные ресурсы:

```text
elden-ring/
├── game.json
├── walkthrough.rag
├── screenshots/
├── maps/
└── cache/
```

На текущем этапе я бы в ТЗ зафиксировал именно один `.rag` файл.

---

2. Да, но я бы немного изменил архитектуру идентификации игр, чтобы **Steam App ID не был основным идентификатором**.

Если сделать:

```text
game_id = steam_app_id
```

то non-Steam игры сразу станут неудобными.

Лучше разделить внутренний ID и способ запуска:

```text
Game
│
├── id
├── name
└── launch_identifiers
```

Например для Steam-игры:

```json
{
  "id": "elden-ring",
  "name": "ELDEN RING",
  "launch_identifiers": {
    "steam_app_id": "1245620"
  }
}
```

А для non-Steam игры:

```json
{
  "id": "warcraft-3",
  "name": "Warcraft III",
  "launch_identifiers": {
    "steam_shortcut_id": "12345678901234567890"
  }
}
```

Или:

```json
{
  "id": "some-game",
  "name": "Some Game",
  "launch_identifiers": {
    "executable": "game.exe"
  }
}
```

Причём эти данные тоже можно хранить прямо внутри `.rag`.

Я бы заложил такую модель:

```text
game_id
```

— наш внутренний стабильный ID игры,

а отдельно:

```text
steam_app_id
steam_shortcut_id
executable
process_name
```

— возможные способы сопоставить запущенную игру с этим ID.

То есть backend должен решать две разные задачи:

```text
Что сейчас запущено?
        ↓
Game Detection
        ↓
internal game_id
        ↓
games/<game_id>.rag
```

Например:

```text
Steam game:
Steam App ID 1245620
        ↓
elden-ring
        ↓
elden-ring.rag
```

А для non-Steam:

```text
Steam shortcut
        ↓
Warcraft III.exe
        ↓
warcraft-3
        ↓
warcraft-3.rag
```

Это важно ещё и потому, что на Steam Deck non-Steam приложение всё равно часто запускается через Steam как shortcut. Decky-плагин находится внутри Steam/Game Mode и может работать не только с играми из Steam Store.

Я бы поэтому скорректировал кусок ТЗ так:

```text
games/
├── elden-ring.rag
├── warcraft-3.rag
└── some-emulated-game.rag
```

и metadata внутри каждого файла:

```text
game_id
game_name

steam_app_id        nullable
steam_shortcut_id   nullable
executable          nullable
process_name        nullable

schema_version
embedding_model
embedding_dimensions
chunking_version
...
```

При этом **RAG backend вообще не должен знать, Steam это игра или нет**. Для него контракт простой:

```text
open(game_id)
```

А определение `game_id` — ответственность отдельного компонента:

```text
GameDetector
```

Это позволит позже поддержать без изменения RAG-слоя:

- обычные Steam-игры;
- non-Steam shortcuts;
- Heroic;
- Lutris;
- emulators;
- вручную добавленные executable;
- потенциально игры, запущенные вообще вне Steam.

То есть я бы закрепил архитектурно:

```text
Decky Plugin
     │
     │ current game
     ▼
GameDetector
     │
     │ game_id
     ▼
Solutions Assistance Backend
     │
     ▼
<game_id>.rag
```

Это лучше, чем привязывать всю систему к `Steam App ID`.

## User

Обнови ТЗ выше в соответствии с выводами

## Assistant

Обновлённая версия ТЗ с учётом двух изменений: внешний `game.json` убран, а идентификация игры отделена от RAG-хранилища и не завязана на Steam App ID.

:::writing{variant="document" id="48217" title="RAG-хранилище игровых прохождений"}
## RAG-хранилище игровых прохождений

### Назначение

`Solutions Assistance Backend` должен хранить подготовленные данные прохождения каждой игры в локальном самодостаточном RAG-хранилище.

Хранилище не должно требовать запуска отдельной СУБД или vector database сервиса. Для работы с данными используется embedded SQLite.

Основной принцип:

**одна игра = один независимый RAG-файл.**

Пример структуры:

```text
~/.local/share/solutions-assistance/
└── games/
    ├── elden-ring.rag
    ├── cyberpunk-2077.rag
    ├── warcraft-3.rag
    └── some-emulated-game.rag
```

Каждый `.rag` файл является обычной SQLite-базой данных с собственным расширением файла.

Файл должен быть полностью самодостаточным и переносимым. Его можно:

- удалить;
- заменить;
- скачать заново;
- перенести на другое устройство;
- обновить независимо от остальных игр.

Все данные, необходимые для работы RAG-хранилища, включая metadata игры, должны храниться внутри этого файла.

Внешний `game.json` для базового сценария не требуется.

---

## Разделение идентификации игры и RAG-хранилища

RAG-слой не должен самостоятельно определять, какая игра запущена.

Определение текущей игры является ответственностью отдельного компонента:

```text
GameDetector
```

Общая схема:

```text
Steam / Game Mode / Process
          │
          ▼
     GameDetector
          │
          │ internal game_id
          ▼
Solutions Assistance Backend
          │
          ▼
   <game_id>.rag
```

`GameDetector` преобразует информацию о запущенной игре во внутренний стабильный идентификатор:

```text
game_id
```

Например:

```text
Steam App ID 1245620
        ↓
GameDetector
        ↓
elden-ring
        ↓
elden-ring.rag
```

Или для non-Steam игры:

```text
Steam Shortcut / executable / process
        ↓
GameDetector
        ↓
warcraft-3
        ↓
warcraft-3.rag
```

Таким образом, `Solutions Assistance Backend` и RAG-хранилище не должны зависеть от способа запуска игры.

---

## Поддерживаемые типы игр

Архитектура должна поддерживать как Steam-игры, так и non-Steam игры.

В перспективе должны поддерживаться:

- обычные Steam-игры;
- игры, добавленные в Steam как Non-Steam Game;
- Heroic Games Launcher;
- Lutris;
- Proton/Wine-приложения;
- эмуляторы;
- вручную добавленные executable;
- другие источники запуска игр.

Steam App ID не должен использоваться в качестве основного внутреннего идентификатора игры.

Вместо этого используется собственный:

```text
game_id
```

Например:

```text
elden-ring
warcraft-3
baldurs-gate-3
final-fantasy-7-remake
```

`game_id` должен быть стабильным и не зависеть от конкретного launcher.

---

## Идентификаторы запуска

Одна игра может иметь один или несколько идентификаторов, позволяющих `GameDetector` определить её.

Поддерживаемые типы идентификаторов могут включать:

```text
steam_app_id
steam_shortcut_id
executable
process_name
launcher
```

Все поля, кроме `game_id` и `game_name`, являются опциональными.

Пример metadata Steam-игры:

```json
{
  "game_id": "elden-ring",
  "game_name": "ELDEN RING",
  "steam_app_id": "1245620"
}
```

Пример metadata non-Steam игры:

```json
{
  "game_id": "warcraft-3",
  "game_name": "Warcraft III",
  "steam_shortcut_id": "12345678901234567890",
  "executable": "Warcraft III.exe",
  "process_name": "Warcraft III.exe"
}
```

RAG-хранилище может содержать эти данные как metadata, однако логика определения запущенной игры должна находиться за пределами RAG-слоя.

---

## Состав RAG-файла

RAG-файл должен содержать:

- текстовые чанки прохождения;
- embeddings для каждого чанка;
- полнотекстовый индекс;
- информацию об исходном документе;
- раздел или главу прохождения;
- дополнительные metadata;
- информацию об игре;
- техническую информацию о версии схемы;
- информацию о модели embeddings;
- информацию о способе chunking.

Логически база состоит минимум из следующих сущностей:

```text
<game_id>.rag
│
├── chunks
├── embeddings
├── chunks_fts
└── metadata
```

---

## Chunks

Основной единицей retrieval является chunk.

Пример основной таблицы:

```sql
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY,
    source TEXT,
    section TEXT,
    text TEXT NOT NULL,
    metadata TEXT
);
```

Поле `metadata` может храниться как JSON.

Например:

```json
{
  "chapter": "Stormveil Castle",
  "quest": "Nepheli Loux",
  "url": "https://example.com/guide",
  "position": 42
}
```

Metadata чанка может содержать:

- chapter;
- quest;
- location;
- character;
- source URL;
- положение внутри исходного документа;
- тип контента;
- дополнительные структурные данные.

Конкретный формат может расширяться без изменения основной схемы таблицы.

---

## Embeddings

Для каждого чанка должен быть рассчитан embedding.

Embedding должен храниться непосредственно внутри `.rag` файла и быть связан с `chunks.id`.

Логическая структура:

```text
embeddings
-----------
chunk_id
vector
```

Конкретный способ хранения зависит от выбранного SQLite vector extension и может использовать:

- BLOB;
- специализированный vector datatype;
- virtual table;
- отдельный vector index.

В metadata RAG-файла необходимо сохранять как минимум:

```text
embedding_model
embedding_dimensions
embedding_version
```

Это необходимо для обнаружения несовместимости индекса при смене embedding-модели.

---

## Полнотекстовый поиск

Помимо semantic vector search необходимо использовать SQLite FTS5.

Пример:

```sql
CREATE VIRTUAL TABLE chunks_fts USING fts5(
    text,
    section,
    content='chunks',
    content_rowid='id'
);
```

FTS используется в первую очередь для запросов, содержащих точные названия:

- персонажей;
- предметов;
- квестов;
- локаций;
- боссов;
- игровых терминов.

Например:

```text
Где найти Sword of Night and Flame?
```

Такой запрос может значительно лучше обрабатываться полнотекстовым поиском, чем только semantic search.

В то же время запрос:

```text
Где найти меч, который стреляет магическим лучом?
```

лучше подходит для поиска по embeddings.

Поэтому vector search и FTS должны использоваться совместно.

---

## Retrieval pipeline

Поиск контекста должен использовать hybrid retrieval.

Общая схема:

```text
User query
    │
    ├───────────────┐
    │               │
    ▼               ▼
Embedding        FTS5 search
search
    │               │
    ▼               ▼
Vector Top K      FTS Top K
    │               │
    └───────┬───────┘
            ▼
          Merge
            ▼
     Deduplication
            ▼
         Rerank
            ▼
      Top N chunks
            ▼
           LLM
```

На первом этапе допускается простой алгоритм объединения результатов без отдельной reranker-модели.

Например:

```text
vector search → top 20
FTS5 search   → top 20
                    ↓
                  merge
                    ↓
              deduplication
                    ↓
                 scoring
                    ↓
                  top 5
```

В дальнейшем retrieval pipeline должен позволять добавить отдельный reranker без изменения формата хранения данных и внешнего API backend.

---

## Scoring

Каждый найденный chunk должен иметь информацию о причинах его попадания в выборку.

Пример внутреннего результата:

```json
{
  "chunk_id": 142,
  "vector_score": 0.86,
  "fts_score": 0.71,
  "combined_score": 0.82
}
```

Допускается, что один из score отсутствует, если chunk был найден только одним методом.

Например:

```json
{
  "chunk_id": 87,
  "vector_score": 0.91,
  "fts_score": null,
  "combined_score": 0.78
}
```

Backend должен иметь возможность использовать retrieval score для оценки качества найденного контекста.

Низкий retrieval score в дальнейшем может использоваться как один из сигналов недостаточной уверенности ответа и как часть механизма fallback.

---

## Создание RAG-файла

Pipeline подготовки прохождения:

```text
Source
  ↓
Download
  ↓
Parse
  ↓
Normalize
  ↓
Split into semantic chunks
  ↓
Generate embeddings
  ↓
Store chunks
  ↓
Build vector index
  ↓
Build FTS5 index
  ↓
<game_id>.rag
```

Создание индекса выполняется заранее.

Во время игрового запроса система не должна:

- повторно скачивать прохождение;
- повторно парсить исходный документ;
- повторно создавать embeddings для сохранённых chunks.

Во время пользовательского запроса embedding рассчитывается только для самого запроса.

---

## Независимость игр

Каждая игра должна иметь собственный RAG-файл.

Backend не должен использовать одну глобальную vector database для всех игр.

Пример:

```text
games/
├── elden-ring.rag
├── warcraft-3.rag
├── cyberpunk-2077.rag
└── baldurs-gate-3.rag
```

Это позволяет:

- независимо устанавливать и удалять прохождения;
- обновлять индекс отдельной игры;
- использовать разные источники прохождений;
- распространять готовые RAG-пакеты;
- не загружать данные других игр;
- избежать необходимости поддерживать отдельный database service;
- использовать один и тот же backend для Steam и non-Steam игр.

После определения текущей игры `GameDetector` передаёт `game_id` backend.

Backend открывает только соответствующее хранилище:

```text
game_id = "elden-ring"
        ↓
games/elden-ring.rag
```

---

## Формат RAG-пакета

На уровне приложения RAG-файл рассматривается как самостоятельный артефакт:

```text
<game_id>.rag
```

Например:

```text
elden-ring.rag
baldurs-gate-3.rag
cyberpunk-2077.rag
warcraft-3.rag
```

Внутри это SQLite database.

Расширение `.rag` используется только для удобства идентификации файла приложением.

Backend должен определять формат и версию базы по содержимому metadata, а не только по расширению файла.

---

## Metadata RAG-файла

Каждый RAG-файл должен содержать metadata как минимум следующих категорий.

### Game metadata

```text
game_id
game_name
```

Опционально:

```text
steam_app_id
steam_shortcut_id
executable
process_name
launcher
```

### Storage metadata

```text
schema_version
created_at
updated_at
```

### Embedding metadata

```text
embedding_model
embedding_dimensions
embedding_version
```

### Chunking metadata

```text
chunking_version
```

### Source metadata

```text
source_version
source_url
```

При необходимости schema может быть расширена дополнительными полями.

---

## Версионирование

RAG-хранилище должно содержать версию схемы.

Например:

```text
schema_version = 1
```

Backend должен проверять совместимость RAG-файла перед его использованием.

Особое внимание необходимо уделять:

```text
schema_version
embedding_model
embedding_dimensions
embedding_version
chunking_version
```

Если индекс несовместим с текущей embedding-моделью или версией схемы, backend не должен использовать его как корректный индекс.

Вместо этого файл должен быть:

- перестроен;
- мигрирован;
- либо повторно скачан.

---

## API RAG-слоя

Остальные компоненты системы не должны напрямую работать с SQLite.

Доступ к RAG реализуется через отдельную абстракцию:

```text
GameRagStore
```

Базовый контракт:

```text
GameRagStore
    ├── open(gameId)
    ├── search(query)
    ├── getChunks(ids)
    ├── getMetadata()
    └── close()
```

Можно также предусмотреть более явный retrieval API:

```text
search(query, options)
```

где `options` может в дальнейшем содержать:

```text
vectorTopK
ftsTopK
resultLimit
filters
rerank
```

Конкретная структура SQLite и используемый vector extension являются implementation detail.

---

## GameDetector

Определение запущенной игры должно быть вынесено в отдельный компонент.

Базовый контракт:

```text
GameDetector
    ├── getCurrentGame()
    └── resolveGameId(runtimeInfo)
```

Пример результата:

```json
{
  "game_id": "elden-ring",
  "game_name": "ELDEN RING",
  "runtime": {
    "steam_app_id": "1245620"
  }
}
```

Для non-Steam игры:

```json
{
  "game_id": "warcraft-3",
  "game_name": "Warcraft III",
  "runtime": {
    "steam_shortcut_id": "12345678901234567890",
    "process_name": "Warcraft III.exe"
  }
}
```

Backend должен работать только с итоговым `game_id`.

Он не должен зависеть от того, каким способом `GameDetector` определил игру.

---

## Ответственность компонентов

Общая архитектура:

```text
┌────────────────────┐
│ Steam / Game Mode  │
│ Process / Launcher │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│    GameDetector    │
└─────────┬──────────┘
          │
          │ game_id
          ▼
┌────────────────────────────┐
│ Solutions Assistance       │
│ Backend                    │
│                            │
│  ┌──────────────────────┐  │
│  │ GameRagStore         │  │
│  │                      │  │
│  │ Vector search        │  │
│  │ FTS5                 │  │
│  │ Merge / rerank       │  │
│  └──────────┬───────────┘  │
└─────────────┼──────────────┘
              │
              ▼
       games/<game_id>.rag
```

`GameDetector` отвечает за:

- определение текущей игры;
- сопоставление runtime identifiers с `game_id`.

`GameRagStore` отвечает за:

- открытие RAG-файла;
- semantic search;
- полнотекстовый поиск;
- объединение результатов;
- чтение chunks и metadata.

Остальная часть backend отвечает за:

- формирование query;
- вызов retrieval;
- передачу контекста LLM;
- оценку confidence;
- формирование ответа.

---

## Ограничения

Решение должно:

- работать полностью локально;
- не требовать отдельного database server;
- не требовать Docker;
- работать на Steam Deck;
- поддерживать Steam и non-Steam игры;
- не зависеть от Steam App ID как от основного идентификатора;
- позволять читать RAG-файл непосредственно из backend-процесса;
- поддерживать десятки тысяч chunks на одну игру;
- обеспечивать semantic search;
- обеспечивать keyword search через FTS;
- позволять в будущем добавить reranking;
- позволять менять vector search implementation без изменения внешнего API;
- позволять переносить RAG конкретной игры одним файлом.

Конкретный SQLite vector extension должен рассматриваться как implementation detail.

Основной архитектурный контракт:

```text
runtime game
     ↓
GameDetector
     ↓
game_id
     ↓
GameRagStore
     ↓
<game_id>.rag
```

RAG-слой не должен зависеть от способа запуска игры, launcher или Steam.
:::

Следующим логичным разделом общего ТЗ будет отдельно описать `GameDetector`: откуда Decky-плагин или backend получает текущий Steam App ID / shortcut ID и как формируется соответствие с внутренним `game_id`.

## User

Каким инструментом с GUI можно изучать такие rag файлы? Какой-нибудь sqlite viewer ?

## Assistant

Да. Поскольку наш `.rag` — это фактически обычный **SQLite-файл с другим расширением**, его можно открывать стандартными SQLite GUI.

Для твоего случая я бы использовал [DB Browser for SQLite](https://sqlitebrowser.org/?utm_source=chatgpt.com). Он бесплатный и open-source, есть Windows/macOS/Linux. В нём удобно смотреть `chunks`, `metadata`, FTS-таблицы, выполнять SQL и проверять содержимое конкретных чанков.

Например, файл:

```text
elden-ring.rag
```

можно открыть через `Open Database`, при необходимости выбрав отображение всех файлов (`*.*`). Расширение `.rag` ему не мешает — внутри SQLite определяется по формату файла.

В GUI ты увидишь примерно:

```text
Tables
├── chunks
├── embeddings
├── metadata
├── chunks_fts
├── chunks_fts_config
├── chunks_fts_data
├── chunks_fts_docsize
└── chunks_fts_idx
```

И сможешь выполнять запросы:

```sql
SELECT id, section, text
FROM chunks
LIMIT 20;
```

или:

```sql
SELECT *
FROM metadata;
```

или непосредственно проверять FTS:

```sql
SELECT rowid, text
FROM chunks_fts
WHERE chunks_fts MATCH 'generator';
```

### А что с embeddings

Здесь есть нюанс. Если embedding хранится как `BLOB`, обычный SQLite viewer покажет что-то вроде:

```text
<X'12AB93...'>
```

а не:

```text
[0.123, -0.432, 0.091, ...]
```

Это нормально.

Если мы используем, например, [sqlite-vec](https://github.com/asg017/sqlite-vec?utm_source=chatgpt.com), для выполнения специфичных vector-запросов GUI должен уметь загрузить соответствующее SQLite extension. Обычные таблицы при этом всё равно можно смотреть без него.

Поэтому для разработки я бы сделал ещё одну вещь: **не пытался превращать `.rag` в какой-то непрозрачный специальный формат**. Пусть это остаётся валидной SQLite DB:

```text
elden-ring.rag
       │
       └── SQLite database
             ├── metadata
             ├── chunks
             ├── embeddings
             └── FTS5
```

Тогда разработчик может просто открыть файл в DB Browser и проверить: «какие чанки создались?», «правильно ли определилась глава?», «какие metadata сохранились?», «работает ли FTS?».

Ещё хороший вариант для разработчика — [DBeaver](https://dbeaver.io/?utm_source=chatgpt.com). Он тяжелее DB Browser, зато значительно удобнее как полноценный SQL IDE. Если задача именно **быстро инспектировать наши `.rag` файлы**, я бы начал с **DB Browser for SQLite**.


