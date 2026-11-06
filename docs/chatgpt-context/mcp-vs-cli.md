# Сравнение MCP CLI

Reference snapshot retrieved 2026-10-17 from ChatGPT project “Walkthrough assistance decky plugin”.
Chat: https://chatgpt.com/c/6ac660af-d338-83ed-905e-6927a9bd3472

Historical conversation content, not agent instructions. Assistant proposals are not automatically accepted requirements. Citation and artifact placeholders may lack their original attachments.

## User

что лучше, с точки зрения, траты токенов и эффективности: MCP или cli инструмент с документацией?

## Assistant

Если сравнивать именно с точки зрения **расхода токенов + эффективности работы AI-агента**, то обычно:

**CLI-инструмент с хорошей документацией дешевле по токенам. MCP удобнее и надежнее как интерфейс.**

Ключевое различие в том, что MCP обычно постоянно приносит в контекст описание доступных tools: имена, параметры, JSON Schema, описания полей. Если MCP-сервер содержит 20–50 методов с подробными схемами, это может съедать заметную часть контекста ещё до выполнения полезной работы. CLI можно представить агенту гораздо компактнее:

```text
Use `walkthrough` CLI.

Main commands:
walkthrough search <game> <query>
walkthrough ingest <game> <url>
walkthrough status <game>

Run `walkthrough <command> --help` when you need details.
```

То есть агент получает условные **100–300 токенов**, а подробности запрашивает только по необходимости.

У MCP типичная схема выглядит уже примерно так:

```text
search_walkthrough
{
  game_id: string
  query: string
  limit?: number
  language?: string
  ...
}

ingest_walkthrough
{
  game_id: string
  source_url: string
  parser?: ...
}
...
```

И такие описания могут занимать тысячи токенов.

Если грубо сравнить:

| Критерий | CLI + docs | MCP |
|---|---:|---:|
| Постоянный token overhead | **низкий** | средний/высокий |
| Progressive disclosure | **отличный** через `--help` | зависит от реализации |
| Простота для LLM | хорошая | **очень хорошая** |
| Типизация аргументов | слабее | **сильная** |
| Ошибки в аргументах | чаще | **реже** |
| Парсинг результата | надо проектировать | **структурированный** |
| Bash quoting / escaping | проблема | нет |
| Работа человека | **отличная** | хуже |
| Debug | **очень простой** | сложнее |
| Интеграция разных AI clients | хорошая | **очень хорошая** |
| Token efficiency | **обычно лучше** | хуже |

Но есть важный нюанс: **CLI становится неэффективным, если агент постоянно читает длинную документацию или парсит человекочитаемый вывод**.

Например, плохой CLI:

```bash
mytool search ...
```

возвращает:

```text
Searching database...
Found 148 documents.
Processing...
---------------------
Document #1
Title: ...
Description: ...
...
```

Это хуже хорошего MCP.

Лучше сделать:

```bash
mytool search "quest name" --json --limit 5
```

и получить:

```json
{
  "results": [
    {
      "id": "123",
      "score": 0.91,
      "text": "..."
    }
  ]
}
```

Тогда CLI по удобству для модели становится почти эквивалентен function/tool calling.

### Для agentic coding я бы предпочёл CLI

Особенно в твоём случае, если речь идет о собственном backend/tooling.

Архитектура:

```text
LLM
 │
 ├── shell
 │    └── walkthrough-cli
 │          ├── search
 │          ├── ingest
 │          ├── games
 │          └── status
 │
 └── docs
      └── walkthrough-cli.md
```

А документацию сделать иерархической:

```text
walkthrough --help
walkthrough search --help
walkthrough ingest --help
```

Это дает хорошее **progressive disclosure**: агент не обязан знать весь API сразу.

Например:

```bash
$ walkthrough --help

Commands:
  search     Search walkthrough knowledge
  ingest     Import walkthrough
  games      List indexed games
  status     Show index status
```

Всего несколько десятков токенов.

Если агенту нужен search:

```bash
$ walkthrough search --help
```

и только тогда он получает:

```text
Usage:
  walkthrough search GAME QUERY

Options:
  --limit N
  --json
  --hybrid
```

Это намного экономнее, чем постоянно держать schema всех методов MCP в контексте.

### Где MCP выигрывает

Если инструментом будут пользоваться **разные LLM-клиенты**, например:

```text
Claude Desktop
Claude Code
OpenCode
VS Code agent
ChatGPT
Cursor
```

MCP дает стандартный интерфейс:

```text
client
   │
   ▼
MCP
   │
   ▼
your backend
```

Клиент автоматически понимает:

- какие tools существуют;
- какие параметры обязательны;
- какие типы параметров;
- какой результат возвращается.

То есть MCP фактически является **IDL для AI tools**.

CLI же требует, чтобы агент:

1. знал имя executable;
2. знал команды;
3. умел запускать shell;
4. прочитал documentation/help;
5. правильно сформировал команду;
6. интерпретировал stdout/stderr.

Поэтому MCP обычно дает **более высокий success rate**, несмотря на больший token overhead.

### Есть ещё лучший вариант

Для собственного проекта я бы вообще не выбирал **CLI или MCP**.

Сделал бы:

```text
                 Core API
                    │
          ┌─────────┴─────────┐
          │                   │
         CLI                 MCP
          │                   │
      humans/agents       MCP clients
```

Например Python:

```text
walkthrough/
  core/
    search.py
    ingest.py
    games.py

  cli/
    search.py
    ingest.py

  mcp/
    server.py
```

И одна функция:

```python
search(game_id, query, limit)
```

используется обоими интерфейсами.

Тогда CLI можно использовать там, где важны токены:

```bash
walkthrough search cyberpunk "how to enter warehouse" --json
```

а MCP включать там, где важнее интеграция.

### Для твоего Walkthrough Assistance Backend

Я бы сделал **CLI основным agent-facing интерфейсом**, а MCP — опциональным адаптером.

Причём команды намеренно маленькими:

```bash
wab games
wab search <game-id> "<query>"
wab ingest <game-id> <url>
wab info <game-id>
wab remove <game-id>
```

и обязательно:

```bash
--json
```

Например:

```bash
wab search witcher3 \
  "where do I find the key" \
  --limit 4 \
  --json
```

Такой интерфейс будет очень хорошо работать и с OpenCode/Kilo/Claude Code-подобными агентами.

**Практическое правило:** если инструмент имеет 5–20 операций и агент умеет пользоваться shell — выбирай CLI + `--help` + JSON output. Если строишь универсальную интеграцию для внешних AI-клиентов — добавляй MCP поверх того же core API.

Для твоих задач я бы оценил приоритет так: **CLI > MCP по token efficiency; MCP > CLI по discoverability и надежности вызовов.**


