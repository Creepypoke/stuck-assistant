# Parser executable

The parser is packaged as a single Linux executable with PyInstaller. Docker
Buildx builds it in an isolated target-platform environment, so the build host
does not need Python or parser dependencies installed.

## Build

Docker Buildx must be available:

```bash
docker buildx create --use
```

Build the default Linux AMD64 binary:

```bash
bash parser/build.sh
```

Pass a Docker platform to build another architecture:

```bash
bash parser/build.sh linux/arm64
```

Artifacts are saved in `parser/dist/` and include the target platform in their
name:

```text
parser/dist/walkthrough-parser-linux-amd64
parser/dist/walkthrough-parser-linux-arm64
```

The target architecture must be supported by the selected Buildx builder. On
hosts without native support, configure Buildx emulation before building.

## Run

```bash
./parser/dist/walkthrough-parser-linux-amd64 < article.html
```
