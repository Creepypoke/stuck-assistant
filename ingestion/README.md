# Ingestion module

The module implements the ingestion path described in [`ARCHITECHTURE.md`](../docs/ARCHITECHTURE.md):

```text
URL -> HTTP download -> Python/trafilatura -> Markdown chunks
    -> llama.cpp embeddings -> games/<game_id>.rag
```

Install the parser dependency in the Python environment used by the backend:

```bash
python3 -m pip install -r parser/requirements.txt
```

Construct `service.Service` with:

* `downloader.HTTPDownloader`, optionally wrapped by `providers.GenericURL`;
* `parser.CommandParser` configured as `python3 parser/walkthrough_parser.py`;
* `chunker.Markdown` with the configured chunk size and overlap;
* `llamacpp.Provider` pointing at the embedding server;
* `rag.Repository` pointing at the `games` directory.

`Service.Ingest` is synchronous, accepts a `context.Context`, and returns the
document ID, chunk count, and generated RAG path. Every stored game is an
independent SQLite file named `<game_id>.rag`.

## CLI

The CLI exposes the ingestion pipeline through one command:

```bash
go run ./cmd/walkthrough-assistance knowledge add \
  test-game https://example.com/walkthrough \
  --parser python3 \
  --parser-arg parser/walkthrough_parser.py
```

By default it expects `walkthrough-parser` in `PATH`, connects to the embedding
server at `http://127.0.0.1:8081`, and stores files in
`~/.local/share/walkthrough-assistance/games`. Run the command with `--help` to
override the parser, embedding model metadata, chunking, timeouts, or storage
directory.
