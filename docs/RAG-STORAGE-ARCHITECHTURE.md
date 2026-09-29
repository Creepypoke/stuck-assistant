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

* удалить;
* заменить;
* скачать заново;
* перенести на другое устройство;
* обновить независимо от остальных игр.

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

* обычные Steam-игры;
* игры, добавленные в Steam как Non-Steam Game;
* Heroic Games Launcher;
* Lutris;
* Proton/Wine-приложения;
* эмуляторы;
* вручную добавленные executable;
* другие источники запуска игр.

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

* текстовые чанки прохождения;
* embeddings для каждого чанка;
* полнотекстовый индекс;
* информацию об исходном документе;
* раздел или главу прохождения;
* дополнительные metadata;
* информацию об игре;
* техническую информацию о версии схемы;
* информацию о модели embeddings;
* информацию о способе chunking.

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

* chapter;
* quest;
* location;
* character;
* source URL;
* положение внутри исходного документа;
* тип контента;
* дополнительные структурные данные.

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

* BLOB;
* специализированный vector datatype;
* virtual table;
* отдельный vector index.

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

* персонажей;
* предметов;
* квестов;
* локаций;
* боссов;
* игровых терминов.

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

* повторно скачивать прохождение;
* повторно парсить исходный документ;
* повторно создавать embeddings для сохранённых chunks.

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

* независимо устанавливать и удалять прохождения;
* обновлять индекс отдельной игры;
* использовать разные источники прохождений;
* распространять готовые RAG-пакеты;
* не загружать данные других игр;
* избежать необходимости поддерживать отдельный database service;
* использовать один и тот же backend для Steam и non-Steam игр.

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

* перестроен;
* мигрирован;
* либо повторно скачан.

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

* определение текущей игры;
* сопоставление runtime identifiers с `game_id`.

`GameRagStore` отвечает за:

* открытие RAG-файла;
* semantic search;
* полнотекстовый поиск;
* объединение результатов;
* чтение chunks и metadata.

Остальная часть backend отвечает за:

* формирование query;
* вызов retrieval;
* передачу контекста LLM;
* оценку confidence;
* формирование ответа.

---

## Ограничения

Решение должно:

* работать полностью локально;
* не требовать отдельного database server;
* не требовать Docker;
* работать на Steam Deck;
* поддерживать Steam и non-Steam игры;
* не зависеть от Steam App ID как от основного идентификатора;
* позволять читать RAG-файл непосредственно из backend-процесса;
* поддерживать десятки тысяч chunks на одну игру;
* обеспечивать semantic search;
* обеспечивать keyword search через FTS;
* позволять в будущем добавить reranking;
* позволять менять vector search implementation без изменения внешнего API;
* позволять переносить RAG конкретной игры одним файлом.

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
