Эксперимент Dino Crisis 1, 5 ноября 2026. Результаты и ограничения — [REPORT.md](REPORT.md). Основные базы лежат в `build-recovered-v3/`; оригинальный английский `.rag` не был доступен, использован текстовый снимок 45 baseline chunks из Langfuse. Не путайте восстановленный контроль с оригинальной базой.

Команды из `backend/`. Требуются Python 3 (stdlib), Go и локальный llama.cpp. Модели уже должны быть скачаны. Временные серверы запускайте отдельно от действующего сервиса, с точными GGUF из manifests:

```sh
llama serve -m /Users/a.esaulenko/.cache/huggingface/hub/models--ggml-org--embeddinggemma-300M-GGUF/snapshots/0f741b5a6585bd53aeb15cd1372c56f2a0f65e12/embeddinggemma-300M-Q8_0.gguf --host 127.0.0.1 --port 18082 --alias embedding --embeddings -c 2048 -np 1 -ngl all

llama serve -m /Users/a.esaulenko/.cache/huggingface/hub/models--unsloth--Qwen3.5-2B-GGUF/snapshots/f6d5376be1edb4d416d56da11e5397a961aca8ae/Qwen3.5-2B-UD-Q4_K_XL.gguf --host 127.0.0.1 --port 18081 --alias assistance-2b --jinja --reasoning off -c 4096 -np 1 -ngl all
```

Сборка из замороженных source chunks/annotations не использует LLM или тестовые вопросы. Обязательно новый output directory; существующие базы не перезаписываются:

```sh
python3 evaluation/situations-v1/prepare.py --out evaluation/situations-v1/rebuild-NEW
python3 evaluation/situations-v1/make_cases.py --build evaluation/situations-v1/rebuild-NEW
python3 evaluation/situations-v1/validate.py --build evaluation/situations-v1/rebuild-NEW
python3 evaluation/situations-v1/audit_context.py --build evaluation/situations-v1/rebuild-NEW
```

`--corpus /path/to/original.rag` позволяет брать сохранённый документ из исходной базы; эта ветка не проверена на отсутствующем оригинале и намеренно отклоняет фрагменты, не совпадающие с сохранённым Markdown. После получения оригинала потребуется проверить selectors и переделать старые разрывы предложений; не выдавайте восстановленные spans за offsets оригинального Markdown. `curate.py` описывает происхождение frozen annotations; повторять его поверх существующего файла не нужно.

Сначала retrieval-only для каждого `control`, `A`, `B`; в примере — B. Смените `--corpus` и `--out` для остальных:

```sh
go run ./cmd/retrieval-eval \
  --corpus evaluation/situations-v1/rebuild-NEW/B/dino-crisis-1.rag \
  --cases evaluation/situations-v1/rebuild-NEW/questions.json \
  --embedding-url http://127.0.0.1:18082 \
  --llama-url http://127.0.0.1:18081 \
  --context-tokens 2400 --modes vector,hybrid \
  --out evaluation/situations-v1/run-NEW-B-retrieval

go run ./cmd/prompt-eval \
  --corpus evaluation/situations-v1/rebuild-NEW/B/dino-crisis-1.rag \
  --cases evaluation/situations-v1/rebuild-NEW/questions.json \
  --embedding-url http://127.0.0.1:18082 \
  --llama-url http://127.0.0.1:18081 --model assistance-2b \
  --context-tokens 2400 --profiles default --modes hybrid \
  --out evaluation/situations-v1/run-NEW-B-local
```

Для oracle добавьте `--kind oracle`, новый `--out`; unanswerable cases будут пропущены. Сохраняйте source spans в gold и заново проверьте ответы: seed не гарантирует одинаковую генерацию. `review.py` материализует только уже просмотренные результаты этого эксперимента и защищён их SHA256; он не оценивает новые ответы автоматически. Индивидуальные решения и rubric находятся в `runs/*/review.json`.

Импорт через существующий exporter, из корня проекта:

```sh
python3 langfuse/pipeline.py import backend/evaluation/situations-v1/run-NEW-B-local \
  --name situations-NEW-B
python3 langfuse/pipeline.py verify backend/evaluation/situations-v1/run-NEW-B-local \
  --name situations-NEW-B
```

Передавайте `--review` только после проверки именно новых ответов; старый review будет отклонён при несовпадении answer/context/prompt hashes. Подключение/секреты берутся штатным exporter из локального `langfuse/.env`, в артефакты эксперимента они не копировались.
