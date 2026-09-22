@Tags(['live'])
library;

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:pulseboard_app/src/api/client.dart';

/// Checks the client against a running reader, which is the only thing that
/// proves the two agree on field names. A mock returns whatever the test
/// author believed the server sends.
///
///   flutter test --tags live --dart-define=PULSEBOARD_BASE_URL=http://127.0.0.1:8080
void main() {
  const base = String.fromEnvironment('PULSEBOARD_BASE_URL',
      defaultValue: 'http://127.0.0.1:8080');

  late PulseboardClient client;

  setUpAll(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    // flutter_test installs an HttpOverrides whose client answers every
    // request with a bodiless 400. These tests are the deliberate exception.
    HttpOverrides.global = null;
  });

  setUp(() => client = PulseboardClient(baseUrl: base));
  tearDown(() => client.close());

  test('feeds and unread parse', () async {
    final feeds = await client.feeds();
    expect(feeds, isNotEmpty);
    expect(feeds.first.title, isNotEmpty);
    expect(await client.unreadCount(), greaterThan(0));
  });

  test('articles parse, with dates and sanitised bodies', () async {
    final items = await client.articles(limit: 5);
    expect(items, isNotEmpty);
    final a = items.first;
    expect(a.title, isNotEmpty);
    expect(a.feedTitle, isNotEmpty);
    // The Nim side formats these explicitly; a struct dump would not parse.
    expect(a.published, isNotNull);

    final full = await client.article(a.id);
    expect(full.id, a.id);
    expect(full.content, isNot(contains('<script')));
  });

  test('read state round-trips', () async {
    final before = await client.unreadCount();
    final a = (await client.articles(limit: 1)).first;

    await client.setRead(a.id);
    expect(await client.unreadCount(), before - 1);
    expect((await client.article(a.id)).isRead, isTrue);

    await client.setRead(a.id, read: false);
    expect(await client.unreadCount(), before);
  });

  test('search reaches FTS5', () async {
    expect(await client.articles(search: 'rust'), isNotEmpty);
  });
}
