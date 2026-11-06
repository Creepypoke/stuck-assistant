Проверка тех же control/A/B на Qwen3.5-4B: [отчёт](REPORT.md), [метрики](comparison.json). Прежние базы и результаты 2B не менялись. Пять новых реальных прогонов — три local (38 вопросов) и два oracle (34 ответимых), default/hybrid, seed 20261104, контекст 2400, output 256 + retry 512.

Из `backend/` запустите временные servers с теми же параметрами. Они не меняют production preset:

```sh
llama serve -m /Users/a.esaulenko/.cache/huggingface/hub/models--ggml-org--embeddinggemma-300M-GGUF/snapshots/0f741b5a6585bd53aeb15cd1372c56f2a0f65e12/embeddinggemma-300M-Q8_0.gguf --host 127.0.0.1 --port 18082 --alias embedding --embeddings -c 2048 -np 1 -ngl all

llama serve -m /Users/a.esaulenko/.cache/huggingface/hub/models--unsloth--Qwen3.5-4B-GGUF/snapshots/e87f176479d0855a907a41277aca2f8ee7a09523/Qwen3.5-4B-UD-Q4_K_XL.gguf --host 127.0.0.1 --port 18083 --alias assistance-4b --jinja --reasoning off -c 4096 -np 1 -ngl all
```

Для полного повторения нужен прежний `situations-v1/` с неизменёнными базами, вопросами и exact-input query caches 2B. Старые полные артефакты доступны в архиве `bin/evaluation-artifacts/situations-v1-recovered-2026-11-05.tar.gz`, hash указан в родительском `artifact.json`. Новые каталоги обязательны:

```sh
mkdir evaluation/situations-v1/4b-repeat-NEW
cp evaluation/situations-v1/4b-2026-11-05/run.py evaluation/situations-v1/4b-repeat-NEW/run.py
python3 evaluation/situations-v1/4b-repeat-NEW/run.py
```

Скрипт проверяет точные веса, сборку, chat/sampling defaults и Go source hashes, копирует исходные query caches и запускает реальный `prompt-eval` последовательно. Существующие output/manifest/results не перезаписывает. Oracle не использует query embeddings. Для одиночного варианта:

```sh
go run ./cmd/prompt-eval \
  --corpus evaluation/situations-v1/build-recovered-v3/B/dino-crisis-1.rag \
  --cases evaluation/situations-v1/build-recovered-v3/questions.json \
  --embedding-url http://127.0.0.1:18082 \
  --llama-url http://127.0.0.1:18083 --model assistance-4b \
  --context-tokens 2400 --profiles default --modes hybrid --seed 20261104 \
  --out evaluation/situations-v1/4b-single-NEW
```

Для точных прежних query-векторов заранее скопируйте `runs/B-local/query-embeddings.json` из родительского каталога в новый `--out`; обёртка `run.py` делает это автоматически. Не копируйте прежние ответы или review. `review.py` защищён SHA256 уже просмотренных результатов и не оценивает новую генерацию автоматически. Новые ответы требуют новой ручной source-проверки.

Импорт из корня проекта через существующий exporter:

```sh
python3 langfuse/pipeline.py import backend/evaluation/situations-v1/4b-single-NEW \
  --name situations-4b-NEW
python3 langfuse/pipeline.py verify backend/evaluation/situations-v1/4b-single-NEW \
  --name situations-4b-NEW
```

`--review` добавляйте только для проверенных именно этих ответов. Локальные секреты Langfuse остаются в штатном `.env`, в архив не включены. После измерений остановите только запущенные для эксперимента servers.
