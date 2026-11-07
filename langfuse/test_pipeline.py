import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import types
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

    def test_reasoning_and_partial_attempts_are_preserved(self):
        record = copy.deepcopy(self.record)
        record['debug']['completion_attempts'][0].update(
            content='partial answer', reasoning_content='model reasoning', reasoning_tokens=240)
        spans, _, _ = p.make_spans(record, self.manifest,
            {'id': 'run', 'name': 'thinking', 'metadata': {}}, 'dataset', 'item', 1000)
        first = next(s for s in spans if s['name'] == 'completion-1')
        attrs = {a['key']: a['value'] for a in first['attributes']}
        self.assertEqual(attrs['langfuse.observation.output']['stringValue'], 'partial answer')
        self.assertEqual(attrs['langfuse.observation.metadata.reasoning_content']['stringValue'], 'model reasoning')
        self.assertEqual(attrs['langfuse.observation.metadata.reasoning_tokens']['intValue'], '240')

    def test_review_must_match_actual_answer_and_prompt(self):
        review = {'records': [dict(self.record, strict_pass=True)]}
        self.assertTrue(p.checked_review(self.record, review)['strict_pass'])
        review['records'][0]['answer'] = 'Different answer'
        with self.assertRaisesRegex(ValueError, 'Stale review'):
            p.checked_review(self.record, review)

    def test_manual_metrics_bind_to_evidence_and_validate_before_network(self):
        entry = dict(self.record, strict_pass=False, task_pass=True,
                     exact_code=True, episode_error=False, exact_sequence=None,
                     answer_language_match=False)
        review = {'records': [entry]}
        self.assertTrue(p.checked_review(self.record, review)['task_pass'])
        self.assertFalse(p.checked_review(self.record, review)['answer_language_match'])
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


class RunTests(unittest.TestCase):
    def test_success_cleans_after_export_and_verify(self):
        with tempfile.TemporaryDirectory() as root, patch.object(p, 'BACKEND', Path(root)):
            def run(*args, **kwargs):
                out = Path(args[0][args[0].index('--out') + 1])
                (out / 'results.json').write_text('[]')
                return types.SimpleNamespace(returncode=0)
            with patch.object(p.sys, 'argv', ['pipeline.py', 'run', '--name', 'test']), \
                 patch.object(p.subprocess, 'run', side_effect=run), \
                 patch.object(p, 'export') as export, patch.object(p, 'verify') as verify:
                self.assertEqual(p.main(), 0)
                out = export.call_args.args[0]
                verify.assert_called_once_with(out, 'test')
                self.assertFalse(out.exists())

    def test_failed_upload_preserves_artifacts(self):
        with tempfile.TemporaryDirectory() as root, patch.object(p, 'BACKEND', Path(root)):
            def run(*args, **kwargs):
                out = Path(args[0][args[0].index('--out') + 1])
                (out / 'results.json').write_text('[]')
                return types.SimpleNamespace(returncode=0)
            with patch.object(p.sys, 'argv', ['pipeline.py', 'run', '--name', 'test']), \
                 patch.object(p.subprocess, 'run', side_effect=run), \
                 patch.object(p, 'export', side_effect=ValueError('offline')) as export:
                with self.assertRaisesRegex(ValueError, 'offline'):
                    p.main()
                self.assertTrue((export.call_args.args[0] / 'results.json').exists())

    def test_rejects_unignored_output(self):
        with patch.object(p.sys, 'argv', ['pipeline.py', 'run', '--name', 'test', '--out', 'unignored']), \
             patch.object(p.subprocess, 'run', return_value=types.SimpleNamespace(returncode=1)):
            with self.assertRaisesRegex(ValueError, 'Git-ignored'):
                p.main()


if __name__ == '__main__':
    unittest.main()
