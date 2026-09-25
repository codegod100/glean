import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:pulseboard_app/src/api/client.dart';
import 'package:pulseboard_app/src/app_state.dart';
import 'package:pulseboard_app/src/screens/home_shell.dart';
import 'package:pulseboard_app/src/theme.dart';

/// A stand-in for the reader, covering the routes the shell touches.
http.Client fakeReader({
  List<String>? seen,
  bool empty = false,
  List<Map<String, Object?>>? articles,
}) {
  return MockClient((req) async {
    seen?.add('${req.method} ${req.url.path}');
    Map<String, String> q = req.url.queryParameters;
    late final Object body;

    switch (req.url.path) {
      case '/feeds':
        if (req.method == 'GET') {
          body = empty
              ? []
              : [
                  {
                    'feed_url': 'https://a.test/feed',
                    'title': 'Feed A',
                    'category': '',
                    'unread': 2,
                    'favicon_url': '',
                  },
                ];
        } else {
          body = {'feed_url': q['url'] ?? '', 'added': 3};
        }
      case '/unread':
        body = {'count': empty ? 0 : 2};
      case '/articles':
        body = articles ?? (empty
            ? []
            : [
                {
                  'id': 1,
                  'feed_url': 'https://a.test/feed',
                  'feed_title': 'Feed A',
                  'title': 'First article',
                  'url': 'https://a.test/1',
                  'author': 'Alice',
                  'summary': '<p>a <b>summary</b></p>',
                  'content': '<p>body</p>',
                  'published': '2026-09-20T10:00:00Z',
                  'is_read': false,
                },
              ]);
      case '/refresh':
        body = {'added': 4, 'errors': []};
      default:
        body = {'ok': true};
    }
    return http.Response(jsonEncode(body), 200,
        headers: {'content-type': 'application/json'});
  });
}

Future<AppState> pump(WidgetTester tester,
    {
      List<String>? seen,
      bool empty = false,
      List<Map<String, Object?>>? articles,
    }) async {
  final state = AppState(
    client: PulseboardClient(
        baseUrl: 'https://reader.test',
        client: fakeReader(seen: seen, empty: empty, articles: articles)),
  );
  await tester.pumpWidget(AppScope(
    state: state,
    child: MaterialApp(
      theme: pulseboardTheme(Brightness.light),
      home: const HomeShell(),
    ),
  ));
  await tester.pumpAndSettle();
  return state;
}

void main() {
  testWidgets('articles and unread counts load on open', (tester) async {
    final seen = <String>[];
    await pump(tester, seen: seen);

    expect(find.text('First article'), findsOneWidget);
    expect(find.text('Feed A'), findsWidgets);
    // Summaries are HTML in most feeds; the tile shows the text they read as.
    expect(find.textContaining('a summary'), findsOneWidget);
    expect(find.textContaining('<b>'), findsNothing);

    expect(seen, contains('GET /articles'));
    expect(seen, contains('GET /feeds'));
  });

  testWidgets('an empty reader says so rather than showing a spinner',
      (tester) async {
    await pump(tester, empty: true);
    expect(find.textContaining('Nothing here'), findsOneWidget);
  });

  testWidgets('the drawer lists feeds with an all-feeds row', (tester) async {
    await pump(tester);
    await tester.tap(find.byTooltip('Open navigation menu'));
    await tester.pumpAndSettle();

    // The server does not send this row; the client adds it.
    expect(find.text('All feeds'), findsOneWidget);
    expect(find.text('Feed A'), findsWidgets);
  });

  testWidgets('refresh reports what arrived', (tester) async {
    final seen = <String>[];
    await pump(tester, seen: seen);

    await tester.tap(find.byTooltip('Refresh'));
    await tester.pumpAndSettle();

    expect(seen, contains('POST /refresh'));
    expect(find.textContaining('4 new'), findsOneWidget);
  });

  testWidgets('refresh reloads the visible articles', (tester) async {
    final seen = <String>[];
    final articles = <Map<String, Object?>>[
      {
        'id': 1,
        'feed_url': 'https://a.test/feed',
        'feed_title': 'Feed A',
        'title': 'First article',
        'url': 'https://a.test/1',
        'author': 'Alice',
        'summary': '',
        'content': '',
        'published': '2026-09-20T10:00:00Z',
        'is_read': false,
      },
    ];
    await pump(tester, seen: seen, articles: articles);

    articles
      ..clear()
      ..add({
        'id': 2,
        'feed_url': 'https://a.test/feed',
        'feed_title': 'Feed A',
        'title': 'New article',
        'url': 'https://a.test/2',
        'author': 'Bob',
        'summary': '',
        'content': '',
        'published': '2026-09-24T10:00:00Z',
        'is_read': false,
      });

    await tester.tap(find.byTooltip('Refresh'));
    await tester.pumpAndSettle();

    expect(seen.where((request) => request == 'GET /articles'), hasLength(2));
    expect(find.text('First article'), findsNothing);
    expect(find.text('New article'), findsOneWidget);
  });

  testWidgets('feed management exposes add and removal controls',
      (tester) async {
    await pump(tester);

    await tester.tap(find.byTooltip('Manage feeds').first);
    await tester.pumpAndSettle();

    expect(find.text('Manage feeds'), findsOneWidget);
    expect(find.text('Add a subscription'), findsOneWidget);
    expect(find.text('Subscriptions'), findsOneWidget);
    expect(find.byTooltip('Remove Feed A'), findsOneWidget);
    expect(find.text('Backup & restore'), findsOneWidget);
  });

  testWidgets('returning from feed management restores the reader',
      (tester) async {
    await pump(tester);

    await tester.tap(find.byTooltip('Manage feeds').first);
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Back'));
    await tester.pumpAndSettle();

    expect(find.text('First article'), findsOneWidget);
  });

  testWidgets('marking an article read updates the count without a refetch',
      (tester) async {
    final state = await pump(tester);
    expect(state.unread, 2);

    // The per-article button, not the app bar's "Mark all read".
    await tester.tap(find.byTooltip('Mark read'));
    await tester.pumpAndSettle();

    // The tile flips locally and the sidebar count follows, rather than the
    // whole list being fetched again.
    expect(state.unread, 1);
  });
}
