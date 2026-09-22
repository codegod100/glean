import 'package:flutter/widgets.dart';

import 'api/client.dart';
import 'models/models.dart';

/// Where the reader is. Override at build time:
///   flutter run --dart-define=GLEAN_BASE_URL=http://10.0.2.2:8080
///
/// 10.0.2.2 is how the Android emulator reaches the host's loopback; a real
/// device needs the machine's LAN address, and the server has no auth, so
/// only do that on a network you trust.
const kDefaultBaseUrl = String.fromEnvironment(
  'GLEAN_BASE_URL',
  defaultValue: 'http://127.0.0.1:8080',
);

/// Feed list and unread counts, shared across screens.
///
/// Articles are not held here: each screen fetches its own, because the
/// filters differ and a shared list would be wrong for whichever screen did
/// not set them.
class AppState extends ChangeNotifier {
  AppState({String baseUrl = kDefaultBaseUrl, GleanClient? client})
      : client = client ?? GleanClient(baseUrl: baseUrl);

  final GleanClient client;

  List<Feed> _feeds = const [];
  int _unread = 0;
  bool _loading = true;
  String? _error;

  List<Feed> get feeds => _feeds;
  int get unread => _unread;
  bool get loading => _loading;
  String? get error => _error;

  /// The feed list with an "all feeds" row in front, which is what the
  /// sidebar shows and the server does not send.
  List<Feed> get sidebar => [Feed.all(_unread), ..._feeds];

  Future<void> load() async {
    _loading = true;
    notifyListeners();
    try {
      _feeds = await client.feeds();
      _unread = await client.unreadCount();
      _error = null;
    } on ApiException catch (e) {
      _error = e.message;
    } on Exception {
      _error = 'Could not reach the reader.';
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  Future<void> addFeed(String url) async {
    await client.addFeed(url);
    await load();
  }

  Future<void> removeFeed(String feedUrl) async {
    await client.removeFeed(feedUrl);
    await load();
  }

  Future<RefreshResult> refresh() async {
    final result = await client.refresh();
    await load();
    return result;
  }

  Future<void> markAllRead({String feedUrl = ''}) async {
    await client.markAllRead(feedUrl: feedUrl);
    await load();
  }

  /// Nudge the counts after a single article changes, without a round trip.
  void adjustUnread(int delta) {
    _unread = (_unread + delta).clamp(0, 1 << 30);
    notifyListeners();
  }

  @override
  void dispose() {
    client.close();
    super.dispose();
  }
}

class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState state, required super.child})
      : super(notifier: state);

  static AppState of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppScope>()!.notifier!;

  static AppState read(BuildContext context) =>
      context.getInheritedWidgetOfExactType<AppScope>()!.notifier!;
}
