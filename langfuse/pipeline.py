#!/usr/bin/env python3
"""Run the Go evaluator and export its real artifacts to local Langfuse v4.

Uses only Python's standard library and OTLP/HTTP JSON. No model calls on import.
"""
import argparse
import base64
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = Path(__file__).resolve().parent
BACKEND = HERE.parent / 'backend'


def encoded(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'))


def digest(value, size=64):
    return hashlib.sha256(encoded(value).encode()).hexdigest()[:size]


def load(path):
    return json.loads(Path(path).read_text())


def environment():
    values = {}
    if (HERE / '.env').exists():
        for line in (HERE / '.env').read_text().splitlines():
            if line.strip() and not line.startswith('#'):
                key, value = line.split('=', 1)
                values[key] = value
    values.update(os.environ)
    return values


class Client:
    def __init__(self):
        env = environment()
        self.host = env.get('LANGFUSE_HOST', 'http://localhost:1488').rstrip('/')
        keys = [env.get('LANGFUSE_PUBLIC_KEY'), env.get('LANGFUSE_SECRET_KEY')]
        if not all(keys):
            raise ValueError('Run python3 langfuse/setup.py first (or set LANGFUSE_* keys).')
        self.auth = 'Basic ' + base64.b64encode(':'.join(keys).encode()).decode()

    def request(self, path, body=None):
        request = urllib.request.Request(self.host + path,
            data=None if body is None else encoded(body).encode(),
            headers={'Authorization': self.auth, 'Content-Type': 'application/json',
                     'x-langfuse-ingestion-version': '4'})
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                raw = response.read()
                result = json.loads(raw) if raw else {}
                if response.status == 207 or result.get('partialSuccess', {}).get('rejectedSpans'):
                    raise ValueError('Langfuse rejected part of the batch: ' + encoded(result))
                return result
        except urllib.error.HTTPError as exc:
            raise ValueError(f'Langfuse {exc.code} {path}: {exc.read().decode()[:1200]}') from exc


def attributes(values):
    def value(v):
        if isinstance(v, bool):
            return {'boolValue': v}
        if isinstance(v, int):
            return {'intValue': str(v)}
        if isinstance(v, float):
            return {'doubleValue': v}
        return {'stringValue': v if isinstance(v, str) else encoded(v)}
    return [{'key': key, 'value': value(v)} for key, v in values.items() if v is not None]


def make_spans(record, manifest, experiment, dataset_id, item_id, start_ns):
    """Reconstruct a timeline from measured durations, not original wall-clock times."""
    debug = record['debug']
    trace_id = digest([experiment['id'], record['case']], 32)
    root_id = digest([trace_id, 'root'], 16)
    common = {
        'langfuse.experiment.id': experiment['id'],
        'langfuse.experiment.name': experiment['name'],
        'langfuse.experiment.dataset.id': dataset_id,
        'langfuse.experiment.item.id': item_id,
        'langfuse.experiment.item.root_observation_id': root_id,
        'langfuse.environment': 'evaluation',
        'langfuse.trace.name': record['case'],
        'langfuse.observation.metadata.mode': record['mode'],
        'langfuse.observation.metadata.profile': record['profile'],
        'langfuse.observation.metadata.context_mode': record['context_mode'],
        'langfuse.observation.metadata.timeline': 'reconstructed from P0 durations; timestamp is import time',
    }
    for key, val in experiment['metadata'].items():
        common['langfuse.experiment.metadata.' + key] = val
    spans = []

    def span(name, offset_ms, duration_ms, extra, parent=root_id, error=''):
        sid = root_id if parent is None else digest([trace_id, name], 16)
        attrs = dict(common, **extra)
        if error:
            attrs.update({'langfuse.observation.level': 'ERROR',
                          'langfuse.observation.status_message': error})
        result = {'traceId': trace_id, 'spanId': sid, 'name': name, 'kind': 1,
                  'startTimeUnixNano': str(start_ns + int(offset_ms * 1e6)),
                  'endTimeUnixNano': str(start_ns + int((offset_ms + max(duration_ms, 0.001)) * 1e6)),
                  'attributes': attributes(attrs),
                  'status': {'code': 2, 'message': error} if error else {'code': 1}}
        if parent is not None:
            result['parentSpanId'] = parent
        spans.append(result)
        return sid

    stages = debug.get('stage_ms', {})
    attempts = debug.get('completion_attempts') or []
    offset = 0.0
    for name, kind, inp, out in (
        ('embedding', 'embedding', debug['request']['question'], manifest.get('embedding_profile')),
        ('retrieval', 'retriever', debug['request'], debug.get('search', {})),
        ('context', 'span', debug.get('retrieved_chunks', []), debug.get('model_request', {})),
    ):
        if name not in stages:
            continue
        duration = stages[name]
        span(name, offset, duration, {'langfuse.observation.type': kind,
             'langfuse.observation.input': encoded(inp), 'langfuse.observation.output': encoded(out)})
        offset += duration
    generation_start = offset
    for i, attempt in enumerate(attempts):
        payload = attempt.get('payload') or {}
        duration = attempt.get('seconds', 0) * 1000
        extra = {
            'langfuse.observation.type': 'generation',
            'langfuse.observation.input': encoded(payload.get('messages', [])),
            'langfuse.observation.model.name': payload.get('model', manifest.get('generation_model')),
            'langfuse.observation.model.parameters': encoded({k: v for k, v in payload.items() if k not in ('model', 'messages')}),
            'langfuse.observation.metadata.finish_reason': attempt.get('finish_reason'),
            'langfuse.observation.metadata.gguf_sha256': manifest.get('generation_runtime', {}).get('gguf_sha256'),
        }
        # The backend does not retain the text of truncated attempts.
        if i == len(attempts) - 1 and not record.get('error'):
            extra['langfuse.observation.output'] = record['answer']
        if attempt.get('usage_available'):
            extra['langfuse.observation.usage_details'] = encoded({
                'input': attempt['prompt_tokens'], 'output': attempt['completion_tokens']})
        span(f'completion-{i + 1}', offset, duration, extra, error=attempt.get('error', ''))
        offset += duration
    total = max(stages.get('total', 0), offset, generation_start + stages.get('generation', 0))
    span('query.Service', 0, total, {
        'langfuse.observation.type': 'chain',
        'langfuse.observation.input': encoded(debug['request']),
        'langfuse.observation.output': record.get('answer', ''),
        'langfuse.experiment.item.expected_output': record['expected'],
        'langfuse.observation.metadata.diagnosis': record.get('diagnosis'),
        'langfuse.observation.metadata.context_sha256': record.get('context_sha256'),
        'langfuse.observation.metadata.prompt_sha256': record.get('prompt_sha256'),
        'langfuse.observation.metadata.retrieved_chunks': encoded(debug.get('retrieved_chunks', [])),
        'langfuse.observation.metadata.manifest': encoded(manifest),
    }, parent=None, error=record.get('error', ''))
    return spans, trace_id, root_id


MANUAL_SCORE_KEYS = ('task_pass', 'episode_error', 'unsupported_addition', 'spoiler', 'exact_code', 'exact_sequence')

def checked_review(record, review):
    if not review:
        return None
    matches = [r for r in review['records'] if all(r.get(k) == record.get(k)
               for k in ('case', 'mode', 'profile', 'context_mode'))]
    if not matches:
        return None
    if len(matches) != 1:
        raise ValueError('Ambiguous review for ' + record['case'])
    entry = matches[0]
    for key in ('answer', 'expected', 'context_sha256', 'prompt_sha256'):
        if key not in entry or entry[key] != record.get(key):
            raise ValueError(f'Stale review: {record["case"]} / {key} differs')
    for key in ('strict_pass',) + MANUAL_SCORE_KEYS:
        if key in entry and entry[key] is not None and not isinstance(entry[key], bool):
            raise ValueError('Manual score must be boolean: ' + key)
    return entry


def export(directory, name, review_path=None, dry_run=False):
    directory = Path(directory).resolve()
    manifest = load(directory / 'manifest.json')
    questions_raw = (directory / 'questions.json').read_bytes()
    if hashlib.sha256(questions_raw).hexdigest() != manifest['cases_sha256']:
        raise ValueError('questions.json does not match manifest cases_sha256')
    questions = {q['id']: q for q in json.loads(questions_raw)}
    records = load(directory / 'results.json')
    review = load(review_path) if review_path else None
    if not records:
        raise ValueError('No evaluator records to export')
    seen = set()
    # Validate before writing anything remotely.
    for r in records:
        key = (r['case'], r['mode'], r['profile'])
        if key in seen or r['case'] not in questions:
            raise ValueError('Duplicate or unknown case: ' + str(key))
        seen.add(key)
        if r['debug']['request']['question'] != questions[r['case']]['question']:
            raise ValueError('Record question differs from questions.json')
        if r['expected'] != questions[r['case']]['expected']:
            raise ValueError('Record expected differs from questions.json')
        checked_review(r, review)
    dataset_name = f'{manifest["game"]}-{manifest["cases_sha256"][:12]}'
    identity = digest([manifest, records, name])
    state_path = directory / ('.langfuse-' + identity[:16] + '.json')
    state = load(state_path) if state_path.exists() else {'start_ns': time.time_ns()}
    if not dry_run:
        state_path.write_text(encoded(state) + '\n')
    client = None if dry_run else Client()
    dataset = {'id': 'dry-run-dataset'} if dry_run else client.request('/api/public/v2/datasets', {
        'name': dataset_name, 'description': 'Gold cases from the Go P0 evaluator',
        'metadata': {'cases_sha256': manifest['cases_sha256']}})
    items = {}
    for case_id, question in questions.items():
        item_id = digest([dataset_name, case_id])
        items[case_id] = item_id
        if not dry_run:
            item = client.request('/api/public/dataset-items', {
                'id': item_id, 'datasetName': dataset_name,
                'input': {'game_id': manifest['game'], 'question': question['question']},
                'expectedOutput': question['expected'],
                'metadata': {k: v for k, v in question.items() if k not in ('question', 'expected')}})
            items[case_id] = item['id']
    runs = {}
    all_scores = []
    for index, record in enumerate(records):
        group = (record['mode'], record['profile'])
        experiment = runs.setdefault(group, {
            'id': digest([identity, group], 32),
            'name': f'{name} / {record["mode"]} / {record["profile"]} / {identity[:8]}',
            'metadata': {'model': manifest.get('generation_model', 'retrieval-only'),
                         'kind': manifest['kind'], 'corpus_sha256': manifest['corpus_sha256'],
                         'manifest_sha256': digest(manifest), 'seed': manifest['seed'],
                         'mode': record['mode'], 'profile': record['profile'],
                         'source': 'Go P0 evaluator; imported artifacts'},
        })
        spans, tid, sid = make_spans(record, manifest, experiment, dataset['id'],
                                    items[record['case']], state['start_ns'] + index * 1000)
        if not dry_run:
            response = client.request('/api/public/otel/v1/traces', {'resourceSpans': [{
                'resource': {'attributes': attributes({'service.name': 'walkthrough-evaluation'})},
                'scopeSpans': [{'scope': {'name': 'walkthrough.p0.import', 'version': '1'}, 'spans': spans}]}]})
            if response.get('partialSuccess', {}).get('errorMessage'):
                print('OTLP warning:', response['partialSuccess']['errorMessage'], file=sys.stderr)
        scores = {'runtime_success': not bool(record.get('error'))}
        # Retrieval correctness does not imply answer correctness. No pseudo gold score.
        if record['context_mode'] != 'oracle' and questions[record['case']].get('evidence'):
            for key in ('candidate_hit', 'candidate_complete', 'context_hit', 'context_complete'):
                scores[key] = record[key]
        reviewed = checked_review(record, review)
        if reviewed is not None:
            scores['gold_strict_pass'] = reviewed['strict_pass']
            for key in MANUAL_SCORE_KEYS:
                if key in reviewed and reviewed[key] is not None:
                    if not isinstance(reviewed[key], bool):
                        raise ValueError('Manual score must be boolean: ' + key)
                    scores[key] = reviewed[key]
        for key, val in scores.items():
            all_scores.append({'id': digest([tid, key]), 'traceId': tid, 'observationId': sid,
                'name': key, 'value': int(val), 'dataType': 'BOOLEAN', 'environment': 'evaluation',
                'timestamp': datetime.fromtimestamp(state['start_ns'] / 1e9, timezone.utc).isoformat(),
                'comment': ((reviewed.get('review_note', '') + '\n' + review.get('rubric', ''))
                            if key in ('gold_strict_pass', 'task_pass', 'episode_error', 'unsupported_addition', 'spoiler', 'exact_code', 'exact_sequence') else 'Deterministic P0 evaluator check; not answer correctness')})
    if not dry_run:
        for offset in range(0, len(all_scores), 50):
            client.request('/api/public/scores', all_scores[offset:offset + 50])
        state.update(dataset=dataset['id'], records=len(records), experiments=list(runs.values()), scores=all_scores)
        state_path.write_text(encoded(state) + '\n')
    print(f'{"Validated" if dry_run else "Exported"}: {len(records)} cases, {len(runs)} experiments, {len(all_scores)} scores')
    for run in runs.values():
        print('  ' + run['name'])
    if not dry_run:
        print(client.host + '/project/walkthrough-backend')
    return len(records)


def verify(directory, name, timeout=30):
    directory = Path(directory).resolve()
    manifest = load(directory / 'manifest.json')
    records = load(directory / 'results.json')
    identity = digest([manifest, records, name])
    state = load(directory / ('.langfuse-' + identity[:16] + '.json'))
    if 'experiments' not in state:
        raise ValueError('Import did not finish; rerun import first')
    client = Client()
    since = datetime.fromtimestamp(state['start_ns'] / 1e9 - 60, timezone.utc).isoformat()
    params = {'fromStartTime': since, 'experimentId': ','.join(r['id'] for r in state['experiments']),
              'fields': 'core,dataset,io,scores', 'limit': 100}
    deadline = time.monotonic() + timeout
    while True:
        rows = []
        query = dict(params)
        while True:
            page = client.request('/api/public/experiment-items?' + urllib.parse.urlencode(query))
            rows.extend(page['data'])
            cursor = page.get('meta', {}).get('cursor')
            if not cursor:
                break
            query['cursor'] = cursor
        actual = {(row['id'], score['name']): score['value'] for row in rows for score in row.get('scores', [])}
        missing = [s for s in state.get('scores', [])
                   if actual.get((s['observationId'], s['name'])) != bool(s['value'])]
        if len(rows) == state['records'] and not missing:
            if any(row.get('expectedOutput') is None or row.get('output') is None for row in rows):
                raise ValueError('Experiment items are missing input/output or gold data')
            print(f'OK: {len(rows)} items, {len(state["experiments"])} experiments, '
                  f'{len(state.get("scores", []))} scores read back from Langfuse')
            return
        if time.monotonic() >= deadline:
            raise ValueError(f'Incomplete ingestion: {len(rows)}/{state["records"]} items; '
                             f'{len(missing)} scores missing or different. Retry verify shortly.')
        time.sleep(2)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    imp = commands.add_parser('import', help='Upload existing Go evaluator artifacts (no model calls)')
    imp.add_argument('directory', type=Path)
    imp.add_argument('--name', required=True, help='Human-readable run name')
    imp.add_argument('--review', type=Path, help='Optional verified P0 manual-review JSON')
    imp.add_argument('--dry-run', action='store_true')
    check = commands.add_parser('verify', help='Read back imported items and every score from Langfuse')
    check.add_argument('directory', type=Path)
    check.add_argument('--name', required=True)
    run = commands.add_parser('run', help='Run real Go evaluator, then upload artifacts, including failures')
    run.add_argument('--name', required=True)
    run.add_argument('--out', required=True, type=Path, help='New artifact directory, relative to shell cwd')
    run.add_argument('--kind', choices=['local', 'oracle', 'retrieval'], default='local')
    run.add_argument('backend_args', nargs=argparse.REMAINDER, help='After --: existing evaluator flags (paths relative to backend)')
    args = parser.parse_args()
    if args.command == 'verify':
        verify(args.directory, args.name)
        return 0
    if args.command == 'import':
        export(args.directory, args.name, args.review, args.dry_run)
        return 0
    out = args.out.resolve()
    backend_args = args.backend_args
    if backend_args[:1] == ['--']:
        backend_args = backend_args[1:]
    if any(a.split('=')[0] in ('--out', '-out', '--kind', '-kind') for a in backend_args):
        raise ValueError('Set --out and --kind before the -- separator')
    command = './cmd/retrieval-eval' if args.kind == 'retrieval' else './cmd/prompt-eval'
    result = subprocess.run(['go', 'run', command, '--out', str(out), '--kind', args.kind] + backend_args, cwd=BACKEND)
    if (out / 'results.json').exists():
        export(out, args.name)
    else:
        print('No results.json: backend failed before producing records.', file=sys.stderr)
    return result.returncode


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, OSError, KeyError) as exc:
        print('Error: ' + str(exc), file=sys.stderr)
        sys.exit(1)
