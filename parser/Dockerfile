# syntax=docker/dockerfile:1

FROM python:3.13-slim AS build

WORKDIR /app

RUN apt-get update \
    && apt-get install --no-install-recommends --yes binutils \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt pyinstaller==6.16.0

COPY walkthrough_parser.py .
RUN pyinstaller --clean --onefile --name walkthrough-parser walkthrough_parser.py

FROM scratch

COPY --from=build /app/dist/walkthrough-parser /walkthrough-parser
