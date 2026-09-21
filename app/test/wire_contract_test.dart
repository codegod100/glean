import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:glean_app/src/api/client.dart';
import 'package:glean_app/src/api/session.dart';

/// Locks the exact field names the Go handlers read.
///
/// These are invisible when wrong: the server happily accepts a request with
/// an unknown field and acts on the zero value, so `mark-all-read` with the
/// wrong key silently marks *everything* read rather than one feed. Each
/// expectation below cites the handler it mirrors.
void main() {
  setUpAll(() => SharedPreferences.setMockInitialValues({}));

  late http.BaseRequest captured;
  late String capturedBody;

  GleanClient clientCapturing({String responseJson = '{}'}) {
    final mock = MockClient((req) async {
      captured = req;
      capturedBody = req.body;
      return http.Response(responseJson, 200,
          headers: {'content-type': 'application/json'});
    });
    return GleanClient(GleanSession(baseUrl: 'https://example.test', client: mock));
  }

  Map<String, String> form() => Uri.splitQueryString(capturedBody);
  Map<String, String> query() => captured.url.queryParameters;

  test('articles filters on "feed" (articles_handler.go:22)', () async {
    await clientCapturing(responseJson: '{"articles":[]}')
        .articles(feedUrl: 'https://ex.test/f', status: 'unread', search: 'go', sortOldest: true);
    expect(query()['feed'], 'https://ex.test/f');
    expect(query().containsKey('feed_url'), isFalse);
    expect(query()['status'], 'unread');
    expect(query()['q'], 'go');
    expect(query()['sort'], 'oldest');
  });

  test('mark-all-read scopes on "feed" (articles_handler.go:384)', () async {
    await clientCapturing().markAllRead(feedUrl: 'https://ex.test/f');
    expect(form()['feed'], 'https://ex.test/f');
    // The dangerous case: an unknown key means an unscoped mark-all-read.
    expect(form().containsKey('feed_url'), isFalse);
  });

  test('add feed posts "feed_url" (feeds_handler.go:128)', () async {
    await clientCapturing(responseJson: '{"subscription":{}}').addFeed('https://ex.test/f');
    expect(form()['feed_url'], 'https://ex.test/f');
  });

  test('remove feed sends "url" (feeds_handler.go:287)', () async {
    await clientCapturing().removeFeed('https://ex.test/f');
    expect(captured.method, 'DELETE');
    expect(form()['url'], 'https://ex.test/f');
  });

  test('retry feed sends "url" (feeds_handler.go:516)', () async {
    await clientCapturing().retryFeed('https://ex.test/f');
    expect(form()['url'], 'https://ex.test/f');
  });

  test('OPML import uploads a file part named "opml" (feeds_handler.go:359)', () async {
    await clientCapturing(responseJson: '{"added":3}').uploadOpml('<opml/>');
    expect(captured.headers['content-type'], startsWith('multipart/form-data'));
    expect(capturedBody, contains('name="opml"'));
  });

  test('settings endpoints set a value rather than toggling '
      '(settings_handler.go:66,85)', () async {
    var c = clientCapturing(responseJson: '{"expanded_view":true}');
    await c.setExpandedView(true);
    expect(form()['expanded_view'], '1');

    c = clientCapturing(responseJson: '{"expanded_view":false}');
    await c.setExpandedView(false);
    expect(form()['expanded_view'], '0');

    c = clientCapturing(responseJson: '{"digest_enabled":true}');
    await c.setDigestEnabled(true);
    expect(form()['digest_enabled'], '1');
  });

  test('dismiss endpoints key on article_url and target_did '
      '(recs_handler.go:7-17)', () async {
    await clientCapturing().dismissArticleRec('https://ex.test/a');
    expect(form()['article_url'], 'https://ex.test/a');

    await clientCapturing().dismissPersonRec('did:plc:abc');
    expect(form()['target_did'], 'did:plc:abc');

    await clientCapturing().dismissFeedRec('https://ex.test/f');
    expect(form()['feed_url'], 'https://ex.test/f');
  });

  test('unsafe methods carry the CSRF token, safe ones do not '
      '(middleware.go:90)', () async {
    final mock = MockClient((req) async {
      captured = req;
      return http.Response('{}', 200, headers: {
        'content-type': 'application/json',
        'set-cookie': 'glean_csrf=tok123; Path=/',
      });
    });
    final session = GleanSession(baseUrl: 'https://example.test', client: mock);
    final c = GleanClient(session);

    await c.me();
    expect(captured.headers.containsKey('X-CSRF-Token'), isFalse);

    await c.refreshFeeds();
    expect(captured.headers['X-CSRF-Token'], 'tok123');
    expect(captured.headers['Cookie'], contains('glean_csrf=tok123'));
  });

  test('error responses surface the server message', () async {
    final mock = MockClient((_) async => http.Response(
        jsonEncode({'error': 'Please enter your handle.'}), 400,
        headers: {'content-type': 'application/json'}));
    final c = GleanClient(GleanSession(baseUrl: 'https://example.test', client: mock));
    await expectLater(
      c.startAuth('x'),
      throwsA(isA<ApiException>().having((e) => e.message, 'message', 'Please enter your handle.')),
    );
  });
}
