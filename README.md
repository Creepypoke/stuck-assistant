# Walkthrough Assistance Backend

Backend хранит и читает готовые `.rag` файлы, выполняет hybrid/vector поиск,
формирует ответы модели и предоставляет локальный API для Decky-плагина.

Подготовка прохождений, разметка и создание баз вынесены в самостоятельный
Go-модуль [data-preparation](../data-preparation/README.md). Команда
`knowledge add`, Python parser и ingestion из backend удалены.

## Сборка и запуск

Требуется Go 1.25+, а для локальных моделей — llama.cpp.

```sh
make -C backend build
backend/bin/walkthrough-assistance --help
backend/bin/walkthrough-assistance-service
```

Бинарники: `backend/bin/walkthrough-assistance` и
`backend/bin/walkthrough-assistance-service`. Makefile создаёт `.env` из `.env.example`, если его ещё нет,
и копирует `llama-cpp/models.ini` в каталог бинарников. Команды выше выполняются из корня
проекта; остальные команды Makefile — из backend или через `make -C backend`.

## Готовые базы

Соберите `.rag` в `data-preparation`, проверьте результат и скопируйте файл
`<game-id>.rag` в каталог `WALKTHROUGH_STORAGE_DIR`.

По умолчанию пример `.env` задаёт `./data/games`: относительный путь сервиса
разрешается от каталога бинарника, то есть `backend/bin/data/games`.
Существующие базы schema v1 сохраняют совместимость. Карточки `situation-v1`
содержат отдельные поисковые тексты и исходные доказательства для генератора.
Поиск открывает SQLite только для чтения.

Query embeddings должны соответствовать embedding-модели и размерности базы.
Текущий формат запроса — исходный текст без добавленных task prefixes.

## CLI и сервис

```sh
backend/bin/walkthrough-assistance ask resident-evil-3-remake 'Что делать дальше?'
backend/bin/walkthrough-assistance service status
backend/bin/walkthrough-assistance service start
backend/bin/walkthrough-assistance service stop
```

CLI обращается к `WALKTHROUGH_SERVICE_URL` (по умолчанию
`http://127.0.0.1:19273`). Доступны `--help` у `ask` и `service`.

Для SteamOS из backend:

```sh
make install
```

Установка выполняется в `~/.local/share/walkthrough-assistance`, создаёт
пользовательский systemd unit и запускает сервис. Parser не устанавливается.

## Конфигурация и API

Настройки runtime находятся в `.env.example`: каталог баз, адрес сервиса,
LLM/embedding endpoint, модель, таймауты, retrieval mode и prompt profile.
Ключ удалённой LLM передаётся через `WALKTHROUGH_LLM_API_KEY`.

Сервис предоставляет `/api/v1/status` и `/ask`. Последний принимает
Decky-совместимый запрос и возвращает SSE со статусами, источниками и ответом.

```json
{
  "game": {"source": "local", "gameId": "resident-evil-3-remake", "displayName": "Resident Evil 3 Remake"},
  "question": "Что делать дальше?",
  "options": {"noSpoilers": true, "answerLength": "short"}
}
```

## Проверки и оценка

```sh
make -C backend test
make -C backend vet
```

Тесты создания фикстур используют writer из соседнего `data-preparation` через
локальный go.mod replace. Это зависимость тестов; runtime-бинарники не импортируют
модуль подготовки. Для разработки и тестирования держите оба модуля рядом.
Команды test/vet/fmt исключают `backend/tmp` согласно AGENTS.md.

Воспроизводимые эксперименты: [evaluation/README.md](evaluation/README.md).
Evaluator работает с готовыми корпусами и не выполняет ingestion.
