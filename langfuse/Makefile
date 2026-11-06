.PHONY: up down status test baseline verify
up:
	python3 setup.py
	docker compose up -d --wait --wait-timeout 300

down:
	docker compose down

status:
	docker compose ps

test:
	python3 -m unittest discover -s . -p 'test_*.py'

baseline:
	python3 pipeline.py import ../backend/tmp/p0-2026-11-04/2b-local --name baseline-2b --review ../backend/evaluation/baselines/p0-2026-11-04/2b-local-review.json
	python3 pipeline.py import ../backend/tmp/p0-2026-11-04/4b-local --name baseline-4b --review ../backend/evaluation/baselines/p0-2026-11-04/4b-local-review.json

verify:
	python3 pipeline.py verify ../backend/tmp/p0-2026-11-04/2b-local --name baseline-2b
	python3 pipeline.py verify ../backend/tmp/p0-2026-11-04/4b-local --name baseline-4b
