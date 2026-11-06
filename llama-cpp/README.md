# llama.cpp server

Host-system launcher for an installed `llama-server`. It uses the model
presets in `models.ini`; models are downloaded and cached using the installed
llama.cpp configuration.

## Run

`llama-server` must be available in `PATH`. From the project root, run:

```bash
bash llama-cpp/start.sh
```

The script runs the server in the foreground with:

```text
llama-server --host 127.0.0.1 --port 8081 --models-preset ./models.ini --models-max 2
```

Both presets (generator and embeddings) stay loaded together, so a question
does not unload one model to load the other. Stop it with `Ctrl+C`.
