import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/testing.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import 'package:glean_app/src/api/session.dart';
import 'package:glean_app/src/app_state.dart';
import 'package:glean_app/src/screens/home_shell.dart';
import 'package:glean_app/src/theme.dart';

/// Fake server covering just the routes the shell touches on startup.
http.Client _fakeServer({required bool signedIn}) {
  return MockClient((req) async {
    final path = req.url.path;
    if (path == '/api/me') {
      return http.Response(
        jsonEncode({
          'user': signedIn
              ? {
                  'did': 'did:plc:abc',
                  'handle': 'reader.bsky.social',
                  'display_name': 'Reader',
                  'avatar_url': '',
                }
              : null,
          'csrf_token': 'tok',
          'has_llm': false,
          'client_id': 'https://example.test/client-metadata',
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (path == '/api/trending/') {
      return http.Response(
        jsonEncode({
          'user': null,
          'trending': [
            {
              'article_id': 1,
              'title': 'A trending article',
              'url': 'https://example.test/a',
              'author': '',
              'summary': '',
              'feed_url': 'https://example.test/feed',
              'feed_title': 'Example',
              'favicon_url': '',
              'like_count': 3,
              'annotation_count': 0,
              'has_liked': false,
            }
          ],
          'scope': 'all',
          'pagination': {
            'page': 1,
            'page_size': 25,
            'has_prev': false,
            'has_next': false,
            'prev_page': 0,
            'next_page': 0,
          },
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    if (path == '/api/dashboard/') {
      return http.Response(
        jsonEncode({
          'user': {'did': 'did:plc:abc', 'handle': 'reader.bsky.social', 'display_name': '', 'avatar_url': ''},
          'subscription_count': 2,
          'unread_count': 7,
          'articles': [],
          'personal_trending': [],
          'global_trending': [],
          'digest_enabled': false,
          'has_llm': false,
          'now': 0,
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }
    return http.Response('{"error":"not found"}', 404,
        headers: {'content-type': 'application/json'});
  });
}

Future<void> _pump(WidgetTester tester, {required bool signedIn}) async {
  SharedPreferences.setMockInitialValues({});
  final state = AppState(
    session: GleanSession(
      baseUrl: 'https://example.test',
      client: _fakeServer(signedIn: signedIn),
    ),
  );
  await state.bootstrap();
  await tester.pumpWidget(
    AppScope(
      state: state,
      child: MaterialApp(theme: gleanTheme(Brightness.light), home: const HomeShell()),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('a signed-out visitor gets Trending and a sign-in affordance',
      (tester) async {
    await _pump(tester, signedIn: false);

    expect(find.text('Sign in'), findsOneWidget);
    expect(find.text('A trending article'), findsOneWidget);
    // Home and Articles would only 401 for a visitor, so they are not offered.
    expect(find.text('Home'), findsNothing);
    expect(find.text('Articles'), findsNothing);
  });

  testWidgets('a signed-in reader gets the full tab set and dashboard counts',
      (tester) async {
    await _pump(tester, signedIn: true);

    expect(find.text('Sign in'), findsNothing);
    expect(find.text('Articles'), findsWidgets);
    expect(find.text('7'), findsOneWidget);
    expect(find.text('unread'), findsOneWidget);
  });
}
