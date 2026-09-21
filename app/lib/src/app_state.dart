import 'package:flutter/widgets.dart';

import 'api/client.dart';
import 'api/responses.dart';
import 'api/session.dart';
import 'models/models.dart';

/// Default server. Override at build time:
///   flutter run --dart-define=GLEAN_BASE_URL=http://10.0.2.2:3000
const kDefaultBaseUrl = String.fromEnvironment(
  'GLEAN_BASE_URL',
  defaultValue: 'https://codegod100--glean-serve.modal.run',
);

/// Whole-app session state: who is signed in, and the client everything else
/// talks through.
///
/// Deliberately a plain ChangeNotifier rather than a state-management package
/// -- the app has exactly one piece of global state (the session) and every
/// screen otherwise owns its own fetch.
class AppState extends ChangeNotifier {
  /// [session] exists so tests can inject a GleanSession backed by a mock
  /// http.Client; production callers only pass a base URL.
  AppState({String baseUrl = kDefaultBaseUrl, GleanSession? session})
      : session = session ?? GleanSession(baseUrl: baseUrl) {
    client = GleanClient(this.session);
  }

  final GleanSession session;
  late final GleanClient client;

  String get baseUrl => session.baseUrl;

  User? _user;
  bool _loading = true;
  bool _hasLlm = false;
  bool _oauthConfigured = false;
  String? _error;

  User? get user => _user;
  bool get signedIn => _user != null;
  bool get loading => _loading;
  bool get hasLlm => _hasLlm;

  /// False when the server has no GLEAN_OAUTH_CLIENT_ID, in which case sign-in
  /// cannot complete and the login screen should say so rather than failing
  /// halfway through a webview.
  bool get oauthConfigured => _oauthConfigured;
  String? get error => _error;

  /// Restore any persisted cookies, then ask the server who we are.
  Future<void> bootstrap() async {
    await session.load();
    await refreshUser();
  }

  Future<void> refreshUser() async {
    _loading = true;
    notifyListeners();
    try {
      final me = await client.me();
      _apply(me);
      _error = null;
    } on ApiException catch (e) {
      _user = null;
      _error = e.message;
    } on Exception {
      _user = null;
      _error = 'Could not reach the server.';
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  void _apply(MeResponse me) {
    _user = me.user;
    _hasLlm = me.hasLlm;
    _oauthConfigured = me.oauthConfigured;
  }

  /// Adopt cookies captured by the OAuth webview and re-check the session.
  Future<void> completeSignIn(Map<String, String> cookies) async {
    await session.adopt(cookies);
    await refreshUser();
  }

  Future<void> signOut() async {
    try {
      await client.logout();
    } on Exception {
      // A failed logout call still means the local session should go.
    }
    await session.clear();
    _user = null;
    notifyListeners();
  }

  @override
  void dispose() {
    session.close();
    super.dispose();
  }
}

/// Makes [AppState] available to the widget tree and rebuilds dependents when
/// it changes.
class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState state, required super.child})
      : super(notifier: state);

  static AppState of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<AppScope>();
    assert(scope != null, 'No AppScope above this widget.');
    return scope!.notifier!;
  }

  /// For callbacks that need the state but should not subscribe to it.
  static AppState read(BuildContext context) {
    final scope = context.getInheritedWidgetOfExactType<AppScope>();
    assert(scope != null, 'No AppScope above this widget.');
    return scope!.notifier!;
  }
}
