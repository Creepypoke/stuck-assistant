import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('pipeline', Path(__file__).with_name('pipeline.py'))
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name)
        self.questions = [{'id': 'door', 'question': 'Where is the code?', 'expected': 'Desk',
                           'evidence': [{'document_id': 1, 'sequence': 2}]}]
        raw = json.dumps(self.questions).encode()
        self.manifest = {'game': 'test', 'kind': 'local', 'seed': 42,
                         'cases_sha256': p.hashlib.sha256(raw).hexdigest(), 'corpus_sha256': 'abc',
                         'generation_model': 'local-2b'}
        self.record = {'case': 'door', 'mode': 'hybrid', 'profile': 'default',
                       'context_mode': 'retrieved', 'answer': 'Desk', 'expected': 'Desk',
                       'context_sha256': 'context', 'prompt_sha256': 'prompt',
                       'candidate_hit': True, 'candidate_complete': True,
                       'context_hit': True, 'context_complete': True,
                       'debug': {'request': {'question': 'Where is the code?'},
                                 'stage_ms': {'embedding': 2, 'retrieval': 3, 'context': 1, 'total': 27},
                                 'completion_attempts': [
                                     {'payload': {'model': 'local-2b', 'max_tokens': 256},
                                      'finish_reason': 'length', 'seconds': .01, 'usage_available': True,
                                      'prompt_tokens': 100, 'completion_tokens': 256},
                                     {'payload': {'model': 'local-2b', 'max_tokens': 512},
                                      'finish_reason': 'stop', 'seconds': .01, 'usage_available': False}]}}
        (self.path / 'questions.json').write_bytes(raw)
        (self.path / 'manifest.json').write_text(json.dumps(self.manifest))
        self.write_records([self.record])

    def write_records(self, records):
        (self.path / 'results.json').write_text(json.dumps(records))

    def test_export_retry_and_idempotent_identity(self):
        calls = []
        class FakeClient:
            host = 'http://localhost:3000'
            def request(self, path, body=None):
                calls.append((path, body))
                if path.endswith('/datasets'):
                    return {'id': 'dataset'}
                if path.endswith('/dataset-items'):
                    return {'id': body['id']}
                return {}
        with patch.object(p, 'Client', FakeClient):
            p.export(self.path, 'test-run')
            first = copy.deepcopy(calls)
            calls.clear()
            p.export(self.path, 'test-run')
        def spans(calls):
            return next(body for path, body in calls if path.endswith('/traces'))['resourceSpans'][0]['scopeSpans'][0]['spans']
        self.assertEqual(spans(first), spans(calls))
        entries = spans(first)
        self.assertEqual(len({s['traceId'] for s in entries}), 1)
        root = next(s for s in entries if 'parentSpanId' not in s)
        self.assertTrue(all(s.get('parentSpanId', root['spanId']) == root['spanId'] for s in entries))
        by_name = {s['name']: {a['key']: a['value'] for a in s['attributes']} for s in entries}
        self.assertNotIn('langfuse.observation.output', by_name['completion-1'])
        self.assertIn('langfuse.observation.output', by_name['completion-2'])
        self.assertNotIn('langfuse.observation.usage_details', by_name['completion-2'])
        scores = next(body for path, body in calls if path.endswith('/scores'))
        self.assertNotIn('gold_strict_pass', [s['name'] for s in scores])

    def test_rejects_changed_gold_and_duplicate_records_before_network(self):
        record = copy.deepcopy(self.record)
        record['expected'] = 'Changed'
        self.write_records([record])
        with self.assertRaisesRegex(ValueError, 'expected differs'):
            p.export(self.path, 'test', dry_run=True)
        self.write_records([self.record, self.record])
        with self.assertRaisesRegex(ValueError, 'Duplicate'):
            p.export(self.path, 'test', dry_run=True)

    def test_review_must_match_actual_answer_and_prompt(self):
        review = {'records': [dict(self.record, strict_pass=True)]}
        self.assertTrue(p.checked_review(self.record, review)['strict_pass'])
        review['records'][0]['answer'] = 'Different answer'
        with self.assertRaisesRegex(ValueError, 'Stale review'):
            p.checked_review(self.record, review)

    def test_manual_metrics_bind_to_evidence_and_validate_before_network(self):
        entry = dict(self.record, strict_pass=False, task_pass=True,
                     exact_code=True, episode_error=False, exact_sequence=None)
        review = {'records': [entry]}
        self.assertTrue(p.checked_review(self.record, review)['task_pass'])
        entry['task_pass'] = 'true'
        path = self.path / 'review.json'
        path.write_text(json.dumps(review))
        with patch.object(p, 'Client') as client:
            with self.assertRaisesRegex(ValueError, 'Manual score must be boolean'):
                p.export(self.path, 'test', review_path=path)
            client.assert_not_called()

    def test_rejects_modified_dataset(self):
        (self.path / 'questions.json').write_text('[]')
        with self.assertRaisesRegex(ValueError, 'cases_sha256'):
            p.export(self.path, 'test', dry_run=True)

    def test_experiments_and_errors_are_separate(self):
        other = copy.deepcopy(self.record)
        other['profile'] = 'strict-ru'
        other['error'] = 'timeout'
        self.write_records([self.record, other])
        self.assertEqual(p.export(self.path, 'test', dry_run=True), 2)


if __name__ == '__main__':
    unittest.main()
