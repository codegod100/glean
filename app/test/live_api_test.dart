@Tags(['live'])
library;

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:glean_app/src/api/client.dart';
import 'package:glean_app/src/api/session.dart';

/// Integration checks against a real Glean server. These assert that the Dart
/// wire models still line up with what the Go API actually emits -- the thing
/// fixture-based unit tests cannot catch when the server changes.
///
///   flutter test --tags live
void main() {
  // flutter_test installs an HttpOverrides whose client answers every request
  // with a bodiless 400, so that tests never accidentally hit the network.
  // These tests are the deliberate exception: clear it to restore real sockets.
  setUpAll(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    HttpOverrides.global = null;
    SharedPreferences.setMockInitialValues({});
  });

  const base = String.fromEnvironment(
    'GLEAN_BASE_URL',
    defaultValue: 'https://codegod100--glean-serve.modal.run',
  );

  late GleanSession session;
  late GleanClient client;

  setUp(() {
    session = GleanSession(baseUrl: base);
    client = GleanClient(session);
  });

  tearDown(() => session.close());

  test('me() parses for a signed-out visitor', () async {
    final me = await client.me();
    expect(me.signedIn, isFalse);
    expect(me.user, isNull);
    // The server issues the CSRF cookie on this call; the session must keep it
    // or every subsequent write would be rejected by the CSRF middleware.
    expect(session.csrfToken, isNotNull);
  });

  test('trending() parses the paginated envelope', () async {
    final t = await client.trending();
    expect(t.scope, isNotEmpty);
    expect(t.pagination.page, greaterThanOrEqualTo(1));
  });

  test('authenticated routes reject a signed-out caller', () async {
    // Proves ApiException carries the server's own message and flags auth
    // failures, which is what the UI keys off to send the user to login.
    await expectLater(
      client.dashboard(),
      throwsA(isA<ApiException>().having((e) => e.isUnauthorized, 'isUnauthorized', isTrue)),
    );
  });
}
