# Walkthrough Assistance

- [`data-preparation`](data-preparation/README.md) — самостоятельный Go CLI:
  локальный TXT/HTML → блоки и YAML-карточки → поисковые представления → `.rag`.
- [`backend`](backend/README.md) — хранение и чтение готовых `.rag`, retrieval,
  ответы модели, локальный API и управление сервисом.
- [`langfuse`](langfuse/README.md) — импорт и просмотр результатов оценки.

Подготовка и сборка базы не входят в runtime backend. Формат обмена — готовый
SQLite `.rag` со схемой v1 и необязательной разметкой `situation-v1`.

```sh
make -C data-preparation build
make -C backend build
```

См. `data-preparation/cli.example.yaml` и
`data-preparation/walkthroughs/resident-evil-3-remake.yaml` для запуска на сохранённом прохождении
Resident Evil 3 Remake. Ключи AI API читаются из переменных окружения, остальные
параметры подготовки — из отдельного YAML-файла для каждого прохождения.
