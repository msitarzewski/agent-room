"""Read-only provider contracts; source records and bearer values are test fixtures."""
from __future__ import annotations

import contextlib
import dataclasses
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch
import urllib.error
import urllib.parse

SOURCE = Path(__file__).resolve().parents[2] / 'adapters/github/agentroom_github.py'
SPEC = importlib.util.spec_from_file_location('agentroom_github', SOURCE)
provider = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = provider
SPEC.loader.exec_module(provider)

SHA = 'a' * 40
OTHER_SHA = 'b' * 40
BASE = '/repos/fixture/source'
NOW = 1792234800.0


def issue(number=1, pull=False, state='open', title='Real source fixture'):
    result = {'id': number + 100, 'number': number, 'title': title, 'body': 'Complete fixture source body', 'state': state, 'updated_at': '2026-10-17T11:00:00Z'}
    if pull:
        result['pull_request'] = {'url': 'https://api.github.com' + BASE + f'/pulls/{number}'}
    return result


def pull(number=1, state='open', merged=False, draft=False, sha=SHA):
    return {'id': 500 + number, 'number': number, 'state': state, 'merged': merged, 'draft': draft, 'head': {'sha': sha}}


class Response:
    def __init__(self, data, headers=None, status=200):
        self.data = json.dumps(data).encode()
        self.headers, self.status = headers or {}, status

    def read(self, limit):
        return self.data[:limit]

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False


class Transport:
    def __init__(self):
        self.records, self.routes = [], {}
        self.issues = []
        self.repository = {'id': 42, 'full_name': 'fixture/source', 'archived': False}
        self.detail = pull()
        self.reviews, self.checks, self.statuses = [], [], []

    def open(self, request, timeout):
        self.records.append((request.method, request.full_url, dict(request.header_items())))
        parsed = urllib.parse.urlsplit(request.full_url)
        if parsed.path in self.routes:
            result = self.routes[parsed.path]
            if callable(result):
                return result(request)
            if isinstance(result, Exception):
                raise result
            return result
        if parsed.path == BASE:
            return Response(self.repository, {'ETag': 'repo-v1'})
        if parsed.path == BASE + '/issues':
            return Response(self.issues)
        if parsed.path == BASE + '/pulls/1':
            return Response(self.detail)
        if parsed.path.endswith('/reviews'):
            return Response(self.reviews)
        if parsed.path.endswith('/check-runs'):
            return Response({'total_count': len(self.checks), 'check_runs': self.checks})
        if parsed.path.endswith('/status'):
            return Response({'sha': parsed.path.split('/')[-2], 'total_count': len(self.statuses), 'statuses': self.statuses})
        raise AssertionError('Unexpected test route')


class CollectorTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='agentroom-github-tests-')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.token = self.root / 'source-token'
        self.token.write_text('fixture-agentroom-token')
        self.token.chmod(0o600)
        self.mapping = provider.Repository('fixture', 'source', 'project', self.token, 42)
        self.config = provider.Config(self.root / 'state', 'github-test-instance', 'http://127.0.0.1:54491/api/v1/ingest', (self.mapping,))
        self.state = provider.State(self.config.state_dir)
        self.addCleanup(lambda: self.state.close() if self.state else None)
        self.transport = Transport()
        self.sender = Mock(return_value=None)
        self.reset_collector()

    def reset_collector(self):
        self.api = provider.GitHub(self.config, self.state, self.transport)
        self.collector = provider.Collector(self.config, self.state, self.api, self.sender)

    def events(self, kind=None):
        events = [json.loads(row[0]) for row in self.state.db.execute('SELECT body FROM queue ORDER BY id')]
        return [event for event in events if kind is None or event['type'] == kind]

    def latest_task(self):
        return self.events('task.observed')[-1]['payload']

    def test_full_and_delta_scans_keep_closed_reopened_identity_and_no_deletion(self):
        self.transport.issues = [issue()]
        self.collector.collect(NOW)
        task = self.latest_task()
        self.assertEqual(task['external_id'], 'repo:42:issue:1')
        self.assertEqual(task['description'].split('Source body:\n')[1], self.transport.issues[0]['body'])
        self.assertEqual(self.events()[0]['source_system'], 'github')
        self.assertEqual(self.events('project.observed')[-1]['payload']['scan_status'], 'complete')
        self.transport.issues = [issue(state='closed')]
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'closed')
        self.assertEqual(self.latest_task()['external_id'], task['external_id'])
        self.assertTrue(any('since=' in url for _, url, _ in self.transport.records))
        self.transport.issues = [issue()]
        self.collector.collect(NOW + 1800)
        self.assertEqual(self.latest_task()['status'], 'open')
        count = len(self.events('task.observed'))
        self.transport.issues = []
        self.collector.collect(NOW + 2700)
        self.assertEqual(len(self.events('task.observed')), count)

    def test_reviews_and_ci_refresh_when_issue_updated_at_and_body_do_not_change(self):
        self.transport.issues = [issue(pull=True)]
        self.collector.collect(NOW)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.transport.checks = [{'id': 1, 'name': 'test', 'head_sha': SHA, 'status': 'completed', 'conclusion': 'failure'}]
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'blocked')
        self.assertIn('branch requirements unknown', self.latest_task()['description'])
        self.transport.checks = [{'id': 2, 'name': 'test', 'head_sha': SHA, 'status': 'completed', 'conclusion': 'success'}]
        self.collector.collect(NOW + 1800)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.assertNotIn('ready', self.latest_task()['description'].lower())

    def test_latest_effective_review_per_user_and_draft_never_infers_ready(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.reviews = [
            {'id': 1, 'user': {'login': 'reviewer'}, 'state': 'CHANGES_REQUESTED', 'commit_id': SHA, 'body': 'Fix this'},
            {'id': 2, 'user': {'login': 'reviewer'}, 'state': 'COMMENTED', 'commit_id': SHA, 'body': 'A comment is not approval'},
        ]
        self.collector.collect(NOW)
        self.assertEqual(self.latest_task()['status'], 'blocked')
        self.transport.reviews.append({'id': 3, 'user': {'login': 'reviewer'}, 'state': 'APPROVED', 'commit_id': SHA, 'body': 'Reviewed'})
        self.transport.detail['draft'] = True
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'draft')
        self.assertNotIn('CHANGES_REQUESTED', self.latest_task()['description'])
        self.assertNotIn('CI: success', self.latest_task()['description'])
        self.assertIn('success is unknown', self.latest_task()['description'])

    def test_closed_is_not_merged_and_historical_checks_are_not_refreshed(self):
        self.transport.issues = [issue(pull=True, state='closed')]
        self.transport.detail = pull(state='closed')
        self.collector.collect(NOW)
        self.assertEqual(self.latest_task()['status'], 'closed')
        self.assertEqual(len(self.transport.records), 3)
        self.assertIn('historical review/CI not refreshed', self.latest_task()['description'])
        self.transport.detail['merged'] = True
        self.transport.issues[0]['updated_at'] = '2026-10-17T11:30:00Z'
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'merged')

    def test_partial_fetch_retains_known_blocker_and_merged_fact(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.checks = [{'id': 1, 'name': 'test', 'head_sha': SHA, 'status': 'completed', 'conclusion': 'failure'}]
        self.collector.collect(NOW)
        self.transport.routes[BASE + '/pulls/1'] = urllib.error.HTTPError('https://api.github.com' + BASE, 503, 'fixture-private-response', {}, None)
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'blocked')
        self.assertIn('STALE', self.latest_task()['description'])
        self.assertEqual(self.events('project.observed')[-1]['payload']['scan_status'], 'partial')
        self.assertEqual(self.state.status()['projects'][0]['last_success'], provider.timestamp(NOW))
        del self.transport.routes[BASE + '/pulls/1']
        self.transport.detail = pull(state='closed', merged=True)
        self.transport.issues = [issue(pull=True, state='closed')]
        self.collector.collect(NOW + 1800)
        self.transport.issues[0]['title'] = 'Changed after merge'
        self.transport.routes[BASE + '/pulls/1'] = urllib.error.URLError('fixture-private-network-detail')
        self.collector.collect(NOW + 2700)
        self.assertEqual(self.latest_task()['status'], 'merged')
        self.assertIn('STALE', self.latest_task()['description'])

    def test_head_change_fetches_only_new_head_checks_and_rejects_mismatches(self):
        self.transport.issues = [issue(pull=True)]
        self.collector.collect(NOW)
        self.transport.detail = pull(sha=OTHER_SHA)
        self.transport.checks = [{'id': 1, 'head_sha': SHA, 'name': 'old test', 'conclusion': 'success'}]
        self.transport.records.clear()
        self.collector.collect(NOW + 900)
        self.assertTrue(any(OTHER_SHA + '/check-runs' in url for _, url, _ in self.transport.records))
        self.assertFalse(any('/commits/' + SHA in url for _, url, _ in self.transport.records))
        self.assertIn('github_check_head_mismatch', self.latest_task()['description'])
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'partial')

    def test_old_check_attempt_failure_does_not_override_latest_success(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.checks = [
            {'id': 1, 'name': 'test', 'head_sha': SHA, 'conclusion': 'failure'},
            {'id': 2, 'name': 'test', 'head_sha': SHA, 'conclusion': 'success'},
        ]
        self.collector.collect(NOW)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.assertNotIn('/failure', self.latest_task()['description'])

    def test_new_queued_attempt_without_start_time_replaces_old_failure(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.checks = [
            {'id': 1, 'name': 'test', 'head_sha': SHA, 'status': 'completed', 'conclusion': 'failure', 'started_at': '2026-10-17T10:00:00Z'},
            {'id': 2, 'name': 'test', 'head_sha': SHA, 'status': 'queued', 'conclusion': None, 'started_at': None},
        ]
        self.transport.statuses = [
            {'id': 1, 'context': 'status', 'state': 'failure', 'created_at': '2026-10-17T10:00:00Z'},
            {'id': 2, 'context': 'status', 'state': 'pending', 'created_at': None},
        ]
        self.collector.collect(NOW)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.assertIn('queued/pending', self.latest_task()['description'])
        self.assertIn('status: pending', self.latest_task()['description'])
        self.assertNotIn('completed/failure', self.latest_task()['description'])

    def test_unchanged_pr_material_is_deduplicated_while_receipts_continue(self):
        self.transport.issues = [issue(pull=True)]
        self.collector.collect(NOW)
        self.collector.collect(NOW + 900)
        self.assertEqual(len(self.events('task.observed')), 1)
        self.assertEqual(len(self.events('project.observed')), 2)

    def test_status_sha_mismatch_never_becomes_current_head_success(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.routes[BASE + '/commits/' + SHA + '/status'] = Response({'sha': OTHER_SHA, 'total_count': 1, 'statuses': [{'context': 'test', 'state': 'success'}]})
        self.collector.collect(NOW)
        self.assertIn('github_status_head_mismatch', self.latest_task()['description'])
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'partial')

    def test_large_review_body_cannot_hide_current_ci_failure_or_other_reviewer(self):
        self.transport.issues = [issue(pull=True)]
        self.transport.reviews = [{'id': 1, 'user': {'login': 'a'}, 'state': 'APPROVED', 'body': 'review source ' * 10000}, {'id': 2, 'user': {'login': 'b'}, 'state': 'CHANGES_REQUESTED'}]
        self.transport.checks = [{'id': 1, 'name': 'critical-test', 'head_sha': SHA, 'conclusion': 'failure'}]
        self.collector.collect(NOW)
        description = self.latest_task()['description']
        self.assertIn('Check critical-test', description)
        self.assertIn('Review by b: CHANGES_REQUESTED', description)
        self.assertIn('observed current-head CI failure', description)
        self.assertIn('truncated', description)

    def test_old_partial_cycle_is_refetched_instead_of_promoted_to_fresh(self):
        self.config = dataclasses.replace(self.config, request_budget=4)
        self.reset_collector()
        self.transport.issues = [issue(pull=True)]
        self.transport.reviews = [{'id': 1, 'user': {'login': 'reviewer'}, 'state': 'APPROVED'}]
        self.collector.collect(NOW)
        self.assertIn('reviews', self.state.meta('github_pull_cycle:project:1'))
        self.config = dataclasses.replace(self.config, request_budget=12)
        self.reset_collector()
        self.transport.reviews = [{'id': 2, 'user': {'login': 'reviewer'}, 'state': 'CHANGES_REQUESTED'}]
        self.collector.collect(NOW + 3601)
        self.assertEqual(self.latest_task()['status'], 'blocked')
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'complete')
        self.assertEqual(sum('/reviews?' in url for _, url, _ in self.transport.records), 2)

    def test_dedup_sequence_retry_and_fingerprints_survive_restart(self):
        self.transport.issues = [issue()]
        self.collector.collect(NOW)
        self.sender.return_value = 'http_503'
        self.collector.deliver(NOW)
        original = self.sender.call_args.args[2]
        events = self.events()
        self.state.close()
        self.state = provider.State(self.config.state_dir)
        self.reset_collector()
        self.sender.return_value = None
        self.collector.deliver(NOW + 6)
        self.assertEqual(self.sender.call_args.args[2], original)
        self.assertEqual(self.events(), [])
        with patch.object(provider.time, 'time_ns', return_value=1):
            self.collector.collect(NOW + 900)
        self.assertEqual(self.events('task.observed'), [])
        self.assertGreater(self.events()[0]['source_sequence'], events[-1]['source_sequence'])
        self.assertEqual(self.events()[0]['payload']['collected_at'], self.events()[0]['occurred_at'])

    def test_cache_and_checkpoint_rollback_together_with_failed_queue_write(self):
        self.transport.issues = [issue()]
        original = self.state.enqueue
        def fail(config, mapping, event_type, payload, now, deduplicate):
            original(config, mapping, event_type, payload, now, deduplicate)
            raise OSError('fixture-private-failure')
        with patch.object(self.state, 'enqueue', side_effect=fail):
            self.collector.collect(NOW)
        self.assertEqual(self.events(), [])
        self.assertIsNone(self.state.meta('github_scan:project'))
        self.assertEqual(self.state.db.execute('SELECT count(*) FROM github_cache').fetchone()[0], 0)
        self.assertEqual(self.state.db.execute('SELECT count(*) FROM observations').fetchone()[0], 0)
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'open')

    def test_repository_remap_cannot_relabel_cached_items_or_fetch_new_repo_details(self):
        self.transport.issues = [issue(pull=True)]
        self.collector.collect(NOW)
        count = len(self.events('task.observed'))
        self.transport.repository['id'] = 99
        self.transport.records.clear()
        self.collector.collect(NOW + 900)
        self.assertEqual(len(self.events('task.observed')), count)
        self.assertEqual(len(self.transport.records), 1)
        self.assertEqual(self.events('project.observed')[-1]['payload']['scan_status'], 'unreadable')
        changed = dataclasses.replace(self.config, projects=(dataclasses.replace(self.mapping, repository_id=99),))
        with self.assertRaises(provider.ConnectorError):
            provider.Collector(changed, self.state)

    def test_explicit_rename_keeps_numeric_identity_and_restarts_old_cursor(self):
        self.transport.issues = [issue()]
        self.collector.collect(NOW)
        renamed = dataclasses.replace(self.mapping, name='renamed')
        self.config = dataclasses.replace(self.config, projects=(renamed,))
        self.transport.routes['/repos/fixture/renamed'] = Response({'id': 42, 'full_name': 'fixture/renamed', 'archived': False})
        self.transport.routes['/repos/fixture/renamed/issues'] = Response([issue(title='Renamed source')])
        self.reset_collector()
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['external_id'], 'repo:42:issue:1')
        self.assertIn('/fixture/renamed/', self.latest_task()['external_url'])

    def test_small_budget_pr_cycle_resumes_across_passes_without_starving_stages(self):
        self.config = dataclasses.replace(self.config, request_budget=4)
        self.reset_collector()
        self.transport.issues = [issue(pull=True)]
        for index in range(3):
            self.collector.collect(NOW + index * 900)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.assertIsNone(self.state.meta('github_pull_cycle:project:1'))
        self.assertEqual(sum('/reviews?' in url for _, url, _ in self.transport.records), 1)
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'partial')
        self.assertIn('not a fresh complete collection', self.latest_task()['description'])

    def test_multiple_repositories_rotate_global_budget_without_starvation(self):
        other = provider.Repository('fixture', 'other', 'other-project', self.token, 84)
        self.config = dataclasses.replace(self.config, request_budget=4, projects=(self.mapping, other))
        self.transport.routes[other.path] = Response({'id': 84, 'full_name': 'fixture/other', 'archived': False})
        self.transport.routes[other.path + '/issues'] = Response([issue(2)])
        self.transport.issues = [issue(pull=True)]
        self.reset_collector()
        self.collector.collect(NOW)
        self.assertNotIn('repo:84:issue:2', [event['payload']['external_id'] for event in self.events('task.observed')])
        self.collector.collect(NOW + 900)
        self.assertIn('repo:84:issue:2', [event['payload']['external_id'] for event in self.events('task.observed')])

    def test_issue_pagination_resumes_after_budget_and_daily_reconciliation_recovers_movement(self):
        self.config = dataclasses.replace(self.config, max_pages=1)
        self.reset_collector()
        def pages(request):
            page = urllib.parse.parse_qs(urllib.parse.urlsplit(request.full_url).query).get('page', ['1'])[0]
            if page == '1':
                link = '<https://api.github.com' + BASE + '/issues?state=all&sort=updated&direction=asc&per_page=20&page=2>; rel="next"'
                return Response([issue(1)], {'Link': link})
            return Response([issue(2)])
        self.transport.routes[BASE + '/issues'] = pages
        self.collector.collect(NOW)
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'partial')
        self.state.close()
        self.state = provider.State(self.config.state_dir)
        self.reset_collector()
        self.collector.collect(NOW + 900)
        self.assertEqual(len(self.events('task.observed')), 2)
        self.assertEqual(self.state.status()['projects'][0]['coverage'], 'full')
        # An item skipped by a non-snapshot moving page is not permanently lost to since.
        self.transport.routes[BASE + '/issues'] = lambda request: Response([] if 'since=' in request.full_url else [issue(3)])
        self.collector.collect(NOW + 1800)
        self.assertNotIn('repo:42:issue:3', [e['payload']['external_id'] for e in self.events('task.observed')])
        self.collector.collect(NOW + 87301)
        self.assertIn('repo:42:issue:3', [e['payload']['external_id'] for e in self.events('task.observed')])

    def test_reviews_paginate_and_resume_partial_page_checkpoint(self):
        self.config = dataclasses.replace(self.config, max_pages=1)
        self.reset_collector()
        self.transport.issues = [issue(pull=True)]
        def reviews(request):
            if 'page=2' in request.full_url:
                return Response([{'id': 2, 'user': {'login': 'reviewer'}, 'state': 'APPROVED'}])
            link = '<https://api.github.com' + BASE + '/pulls/1/reviews?per_page=100&page=2>; rel="next"'
            return Response([{'id': 1, 'user': {'login': 'reviewer'}, 'state': 'CHANGES_REQUESTED'}], {'Link': link})
        self.transport.routes[BASE + '/pulls/1/reviews'] = reviews
        self.collector.collect(NOW)
        self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'partial')
        self.collector.collect(NOW + 900)
        self.assertEqual(self.latest_task()['status'], 'review')
        self.assertIn('APPROVED', self.latest_task()['description'])
        self.assertNotIn('CHANGES_REQUESTED', self.latest_task()['description'])

    def test_initial_rate_limit_retry_after_and_reset_are_durable(self):
        for headers, wait in [({'Retry-After': '120'}, 120), ({'X-RateLimit-Remaining': '0', 'X-RateLimit-Reset': str(NOW + 180)}, 181)]:
            with self.subTest(headers=headers):
                with self.state.db:
                    self.state.set_meta('github_next_request', 0)
                self.transport.routes[BASE] = urllib.error.HTTPError('https://api.github.com' + BASE, 429, 'fixture-private-rate-body', headers, None)
                self.collector.collect(NOW)
                self.assertEqual(self.state.meta('github_next_request'), NOW + wait)
                count = len(self.transport.records)
                self.collector.collect(NOW + wait - 1)
                self.assertEqual(len(self.transport.records), count)
                self.assertEqual(self.state.status()['projects'][0]['scan_status'], 'unreadable')

    def test_long_body_truncation_preserves_current_ci_facts_and_safe_status_output(self):
        source = issue(pull=True)
        source['body'] = 'source text ' * 10000
        self.transport.issues = [source]
        self.collector.collect(NOW)
        self.assertLessEqual(len(self.latest_task()['description']), provider.BODY_LIMIT)
        self.assertIn('CI: no current-head', self.latest_task()['description'])
        self.assertIn('truncated', self.latest_task()['description'])
        output = provider.canonical(self.state.status())
        self.assertNotIn('source text', output)
        self.assertNotIn('fixture-agentroom-token', output)

    def test_source_provider_instance_and_private_state_cannot_be_reused(self):
        with self.assertRaises(provider.ConnectorError):
            self.state.bind_source('other-instance', 'github')
        with self.assertRaises(provider.ConnectorError):
            self.state.bind_source(self.config.instance_id, 'sentry')
        self.config.state_dir.chmod(0o755)
        with self.assertRaises(provider.ConnectorError):
            provider.State(self.config.state_dir)

    def test_etag_304_uses_cached_body_and_link_without_losing_items(self):
        self.api.repo = self.mapping
        self.api.begin(NOW)
        with self.state.db:
            first, link = self.api.get(BASE)
        self.transport.routes[BASE] = urllib.error.HTTPError('https://api.github.com' + BASE, 304, 'Not Modified', {}, None)
        self.api.begin(NOW + 900)
        with self.state.db:
            second, second_link = self.api.get(BASE)
        self.assertEqual((second, second_link), (first, link))
        headers = {key.lower(): value for key, value in self.transport.records[-1][2].items()}
        self.assertEqual(headers['if-none-match'], 'repo-v1')
        self.assertEqual(headers['x-github-api-version'], provider.API_VERSION)
        self.assertNotIn('authorization', headers)

    def test_optional_private_token_is_explicit_and_never_sent_to_redirect_target(self):
        github_token = self.root / 'github-token'
        github_token.write_text('fixture-github-private-token')
        github_token.chmod(0o600)
        self.config = dataclasses.replace(self.config, github_token_file=github_token)
        self.reset_collector()
        self.transport.routes[BASE] = urllib.error.HTTPError('https://api.github.com' + BASE, 302, 'fixture-private-redirect-body', {'Location': 'https://outside.invalid/private'}, None)
        self.collector.collect(NOW)
        self.assertEqual(len(self.transport.records), 1)
        self.assertEqual(self.transport.records[0][0], 'GET')
        self.assertNotIn('fixture-private', provider.canonical(self.state.status()))
        self.assertTrue(self.transport.records[0][1].startswith('https://api.github.com/repos/fixture/source'))
        github_token.chmod(0o644)
        self.collector.collect(NOW + 900)
        self.assertEqual(len(self.transport.records), 1)
        with patch.object(provider.urllib.request, 'build_opener', return_value=self.transport) as opener:
            provider.GitHub(self.config, self.state)
        self.assertIsInstance(opener.call_args.args[0], provider.urllib.request.ProxyHandler)
        self.assertEqual(opener.call_args.args[0].proxies, {})
        self.assertIsInstance(opener.call_args.args[1], provider.NoRedirect)

    def test_unsafe_pagination_and_api_paths_are_rejected_before_authenticated_get(self):
        self.api.repo = self.mapping
        self.api.begin(NOW)
        for path in ['https://outside.invalid/private', '/repos/other/source/issues', BASE + '/contents/private', BASE + '/issues?access_token=private']:
            with self.assertRaises(provider.FetchError):
                self.api.get(path)
        for target in ['https://outside.invalid' + BASE + '/issues?per_page=20&page=2', 'https://user:private@api.github.com' + BASE + '/issues?per_page=20&page=2', 'https://api.github.com' + BASE + '/issues?per_page=20&since=changed&page=2']:
            with self.assertRaises(provider.FetchError):
                self.api.next_page(BASE + '/issues?per_page=20', '<' + target + '>; rel="next"')
        self.assertEqual(self.transport.records, [])

    def test_response_limit_and_pagination_loop_remain_visible_without_checkpoint(self):
        self.api.repo = self.mapping
        self.api.begin(NOW)
        oversize = Response(None)
        oversize.data = b'x' * (8 * 1024 * 1024 + 1)
        self.transport.routes[BASE] = oversize
        with self.assertRaisesRegex(provider.FetchError, 'response_limit'):
            with self.state.db:
                self.api.get(BASE)
        self.assertEqual(self.state.db.execute('SELECT count(*) FROM github_cache').fetchone()[0], 0)
        with self.assertRaises(provider.FetchError):
            self.api.next_page(BASE + '/issues?per_page=20&page=2', '<https://api.github.com' + BASE + '/issues?per_page=20&page=2>; rel="next"')

    def test_source_delivery_failures_quarantine_exact_events_without_deleting(self):
        self.transport.issues = [issue()]
        self.collector.collect(NOW)
        original = [event['source_event_id'] for event in self.events()]
        self.sender.return_value = 'http_400'
        self.collector.deliver(NOW)
        self.assertEqual(self.state.status()['quarantined'], 2)
        self.assertEqual([event['source_event_id'] for event in self.events()], original)
        self.assertEqual(self.state.status()['pending'], 0)

    def test_cli_once_and_watch_use_current_status_and_honest_delivery_exit(self):
        stopped = Mock()
        stopped.is_set.side_effect = [False, True]
        status = self.state.status()
        status.update({'collection_error': None, 'pending': 1})
        output = io.StringIO()
        with patch.object(provider, 'load_config', return_value=self.config), patch.object(provider, 'Collector') as factory, patch.object(provider.State, 'status', return_value=status), patch.object(provider.signal, 'signal'), contextlib.redirect_stdout(output):
            with patch.object(sys, 'argv', ['connector', '--config', 'fixture.toml', 'once']):
                self.assertEqual(provider.main(), 2)
            factory.return_value.collect.assert_called_once()
            factory.return_value.deliver.assert_called_once()
        with patch.object(provider, 'load_config', return_value=self.config), patch.object(provider, 'Collector'), patch.object(provider.signal, 'signal'), patch.object(provider.threading, 'Event', return_value=stopped), patch.object(sys, 'argv', ['connector', '--config', 'fixture.toml', 'watch']):
            self.assertEqual(provider.main(), 0)
        stopped.wait.assert_called_once()


class ConfigTest(unittest.TestCase):
    def test_cli_status_is_readonly_and_config_uses_no_implicit_tokens(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            path = root / 'config.toml'
            config = f'''[connector]
state_dir = {json.dumps(str(root / 'absent'))}
instance_id = "test-instance"
ingest_url = "http://127.0.0.1:54491/api/v1/ingest"
[[repositories]]
owner = "fixture"
name = "source"
project_id = "project"
repository_id = 42
token_file = {json.dumps(str(root / 'agentroom-token'))}
'''
            path.write_text(config)
            loaded = provider.load_config(path)
            self.assertEqual(loaded.request_budget, 12)
            self.assertEqual(loaded.poll_seconds, 900)
            self.assertIsNone(loaded.github_token_file)
            for original, invalid in [('owner = "fixture"', 'owner = "../other"'), ('repository_id = 42', 'repository_id = 0'), ('http://127.0.0.1:54491', 'http://outside.invalid'), ('project_id = "project"', 'project_id = ""')]:
                path.write_text(config.replace(original, invalid))
                with self.assertRaises(provider.ConnectorError):
                    provider.load_config(path)
            path.write_text(config)
            output = io.StringIO()
            with patch.object(sys, 'argv', ['connector', '--config', str(path), 'status']), contextlib.redirect_stdout(output):
                self.assertEqual(provider.main(), 0)
            self.assertFalse(json.loads(output.getvalue())['initialized'])
            self.assertFalse((root / 'absent').exists())


if __name__ == '__main__':
    unittest.main()
