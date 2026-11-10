#!/usr/bin/env python3
"""Generate comparable RAG answers, sequentially, using the existing Go evaluator."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time
import urllib.request

HERE = Path(__file__).resolve().parent
BACKEND = HERE.parent
REPO = BACKEND.parent
BASE = HERE / 'model-comparison-2026-11-07'
DATA = HERE / 'situations-v1'
PROFILES = {
    'qwen2b': (1.0, 20, .95, 0, 1.5),
    'minicpm5-1b': (.9, 40, .95, 0, 0),
    'gemma4-e2b': (1.0, 64, .95, 0, 0),
    'jamba3b': (.6, 40, .95, 0, 0),
}
CONTROLS = ('gemma3-4b', 'phi4-mini')


def save(path, value):
    tmp = path.with_suffix(path.suffix + '.new')
    tmp.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    os.replace(tmp, path)


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(4 * 1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def request(port, endpoint, payload=None, timeout=2):
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(f'http://127.0.0.1:{port}{endpoint}', data=data,
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=timeout) as response:
        return json.load(response)


def start(cmd, log_path, port):
    log = log_path.open('x')
    proc = subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT)
    log.close()
    try:
        for _ in range(180):
            if proc.poll() is not None:
                raise RuntimeError(f'Server exited: {log_path}')
            try:
                return proc, request(port, '/props')
            except (OSError, ValueError):
                time.sleep(1)
        raise TimeoutError(f'Server not ready: {log_path}')
    except BaseException:
        stop(proc)
        raise


def stop(proc):
    if proc and proc.poll() is None:
        proc.terminate()
        try:
            proc.wait(timeout=20)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True, help='New, non-existing series directory')
    parser.add_argument('--models', nargs='+', choices=[*PROFILES, *CONTROLS], default=list(PROFILES))
    parser.add_argument('--kinds', nargs='+', choices=['local', 'oracle'], default=['local', 'oracle'])
    parser.add_argument('--embedding-port', type=int, default=18082)
    parser.add_argument('--model-port', type=int, default=18083)
    parser.add_argument('--langfuse', action='store_true', help='Import raw results (no correctness scores)')
    args = parser.parse_args()
    if len(set(args.models)) != len(args.models) or len(set(args.kinds)) != len(args.kinds):
        parser.error('Duplicate models or kinds')
    weights = {w['name']: w for w in json.loads((BASE / 'weights.json').read_text())}
    baseline = json.loads((BASE / 'runs/qwen2b-local/manifest.json').read_text())
    embedding = Path(baseline['embedding_runtime']['model_path'])
    inputs = [embedding, DATA / 'build-recovered-v3/questions.json',
              DATA / 'build-recovered-v3/B/dino-crisis-1.rag',
              DATA / 'runs/B-local/query-embeddings.json', BACKEND / 'bin/prompt-eval']
    for name in args.models:
        weight = weights[name]
        path = Path(weight['path'])
        if not path.is_file() or digest(path) != weight['sha256']:
            raise SystemExit(f'Missing or changed model weights: {path}')
    for path in inputs:
        if not path.is_file():
            raise SystemExit(f'Missing input: {path}')
    if not shutil.which('llama'):
        raise SystemExit('llama executable not found')
    for port in (args.embedding_port, args.model_port):
        import socket
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1', port)) == 0:
                raise SystemExit(f'Port {port} is already in use')
    if args.embedding_port == args.model_port:
        parser.error('Ports must differ')
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    (out / 'runs').mkdir()
    (out / 'logs').mkdir()
    state = {'status': 'running', 'models': args.models, 'kinds': args.kinds, 'jobs': []}
    save(out / 'status.json', state)
    save(out / 'configuration.json', {'models': [weights[n] for n in args.models],
         'sampling': {n: PROFILES.get(n) for n in args.models}, 'context': 16384,
         'max_tokens': 4096, 'retry_max_tokens': 8192, 'seed': 20261104,
         'input_sha256': {str(p): digest(p) for p in inputs}, 'langfuse': args.langfuse})
    emb = model_proc = None
    def interrupt(signum, frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupt)
    try:
        emb_cmd = ['llama', 'serve', '-m', str(embedding), '--host', '127.0.0.1', '--port', str(args.embedding_port),
                   '--alias', 'embedding', '--embeddings', '-c', '2048', '-np', '1', '-ngl', 'all']
        emb, _ = start(emb_cmd, out / 'logs/embedding.log', args.embedding_port)
        for name in args.models:
            thinking = name in PROFILES
            temp, top_k, top_p, min_p, penalty = PROFILES.get(name, (.2, 40, .95, 0, 0))
            cmd = ['llama', 'serve', '-m', weights[name]['path'], '--host', '127.0.0.1',
                   '--port', str(args.model_port), '--alias', name, '--jinja', '-c', '16384',
                   '-np', '1', '-ngl', 'all', '--top-k', str(top_k), '--top-p', str(top_p),
                   '--min-p', str(min_p), '--presence-penalty', str(penalty)]
            if thinking:
                cmd += ['--reasoning', 'on', '--reasoning-format', 'deepseek']
            model_proc, props = start(cmd, out / f'logs/{name}-server.log', args.model_port)
            try:
                payload = {'model': name, 'messages': [{'role': 'user', 'content': 'Ответь: сколько будет 2 + 2?'}],
                           'max_tokens': 512, 'temperature': temp, 'seed': 20261104,
                           'chat_template_kwargs': {'enable_thinking': thinking}}
                template = request(args.model_port, '/apply-template', payload, 120)
                response = request(args.model_port, '/v1/chat/completions', payload, 120)
                save(out / f'{name}-preflight.json', {'command': cmd, 'props': props, 'payload': payload,
                                                     'template': template, 'response': response})
                message = response['choices'][0]['message']
                observed = bool(message.get('reasoning_content') or message.get('reasoning'))
                if thinking and not observed:
                    raise RuntimeError(f'{name}: reasoning not observed in preflight; see saved probe')
                if not thinking and (observed or '<think>' in message.get('content', '')):
                    raise RuntimeError(f'{name}: unexpected reasoning in control preflight')
                for kind in args.kinds:
                    run = out / 'runs' / f'{name}-{kind}'
                    run.mkdir()
                    if kind == 'local':
                        shutil.copyfile(DATA / 'runs/B-local/query-embeddings.json', run / 'query-embeddings.json')
                    eval_cmd = [str(BACKEND / 'bin/prompt-eval'), '--kind', kind,
                        '--corpus', str(inputs[2]), '--cases', str(inputs[1]),
                        '--embedding-url', f'http://127.0.0.1:{args.embedding_port}',
                        '--llama-url', f'http://127.0.0.1:{args.model_port}', '--model', name,
                        '--context-tokens', '2400', '--profiles', 'default', '--modes', 'hybrid',
                        '--seed', '20261104', '--out', str(run), '--max-tokens', '4096',
                        '--retry-max-tokens', '8192', '--temperature', str(temp)]
                    if thinking:
                        eval_cmd.append('--enable-thinking')
                        if name == 'jamba3b': eval_cmd.append('--allow-reasoning')
                    else:
                        eval_cmd.append('--verified-non-thinking')
                    job = {'model': name, 'kind': kind, 'status': 'running', 'command': eval_cmd}
                    state['jobs'].append(job)
                    save(out / 'status.json', state)
                    print(f'Running {name} {kind}', flush=True)
                    with (out / f'logs/{name}-{kind}.log').open('x') as log:
                        rc = subprocess.run(eval_cmd, cwd=BACKEND, stdout=log, stderr=subprocess.STDOUT).returncode
                    result = run / 'results.json'
                    records = json.loads(result.read_text()) if result.exists() else []
                    cases = json.loads(inputs[1].read_text())
                    expected = {c['id'] for c in cases if kind == 'local' or c.get('evidence')}
                    ids = [r['case'] for r in records]
                    complete = len(ids) == len(expected) and set(ids) == expected
                    job.update(exit_code=rc, records=len(records), errors=sum(bool(r.get('error')) for r in records),
                               status='generated' if complete else 'incomplete',
                               results_sha256=digest(result) if result.exists() else None)
                    save(out / 'status.json', state)
                    if not complete:
                        raise RuntimeError(f'Incomplete results: {name} {kind}; see log')
                    if args.langfuse:
                        with (out / f'logs/{name}-{kind}-import.log').open('x') as log:
                            imp = subprocess.run([sys.executable, str(REPO / 'langfuse/pipeline.py'), 'import', str(run),
                                '--name', f'{out.name}-{name}-{kind}'], cwd=REPO, stdout=log, stderr=subprocess.STDOUT)
                        job['langfuse_import_exit_code'] = imp.returncode
                        save(out / 'status.json', state)
                        if imp.returncode: raise RuntimeError(f'Langfuse import failed: {name} {kind}')
            finally:
                stop(model_proc)
                model_proc = None
        state['status'] = 'generated'
    except KeyboardInterrupt:
        state['status'] = 'stopped_user'
        raise
    except Exception as exc:
        state.update(status='failed', error=str(exc))
        raise
    finally:
        stop(model_proc)
        stop(emb)
        save(out / 'status.json', state)
    print(f'Generation complete; manual source-based review required: {out}')


if __name__ == '__main__':
    main()
