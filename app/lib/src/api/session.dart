import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

/// Cookie-backed session against the Glean API.
///
/// The Go server authenticates with a session cookie and guards unsafe methods
/// with a double-submit CSRF token: the value lives in the `glean_csrf` cookie
/// and must be echoed back in the `X-CSRF-Token` header (internal/server/
/// middleware.go). Dart's http package has no cookie jar, so this class is one
/// -- deliberately minimal, since the server sets only a handful of cookies on
/// a single origin.
class GleanSession {
  GleanSession({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;
  final Map<String, String> _cookies = {};

  static const _prefsKey = 'glean_cookies';
  static const _csrfCookie = 'glean_csrf';

  bool get hasSession => _cookies.isNotEmpty;
  String? get csrfToken => _cookies[_csrfCookie];

  /// Cookies survive restarts so a signed-in user stays signed in. They are
  /// session credentials, so callers on mobile may prefer secure storage; this
  /// uses shared_preferences to keep the dependency surface small.
  Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getStringList(_prefsKey) ?? const [];
    for (final entry in raw) {
      final i = entry.indexOf('=');
      if (i > 0) _cookies[entry.substring(0, i)] = entry.substring(i + 1);
    }
  }

  Future<void> _persist() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(
      _prefsKey,
      _cookies.entries.map((e) => '${e.key}=${e.value}').toList(),
    );
  }

  Future<void> clear() async {
    _cookies.clear();
    await _persist();
  }

  /// Adopt cookies captured elsewhere -- notably the webview that completes the
  /// ATProto OAuth redirect, which is where the session cookie is first issued.
  Future<void> adopt(Map<String, String> cookies) async {
    _cookies.addAll(cookies);
    await _persist();
  }

  Map<String, String> _headers({bool unsafe = false, Map<String, String>? extra}) {
    final h = <String, String>{'Accept': 'application/json'};
    if (_cookies.isNotEmpty) {
      h['Cookie'] = _cookies.entries.map((e) => '${e.key}=${e.value}').join('; ');
    }
    // Only unsafe methods are CSRF-checked; sending it on GET is harmless but
    // noisy, and mirrors what the web client does.
    if (unsafe && csrfToken != null) h['X-CSRF-Token'] = csrfToken!;
    if (extra != null) h.addAll(extra);
    return h;
  }

  void _absorb(http.BaseResponse res) {
    final raw = res.headers['set-cookie'];
    if (raw == null) return;
    for (final c in _splitSetCookie(raw)) {
      try {
        final cookie = Cookie.fromSetCookieValue(c);
        // An expiry in the past is the server deleting the cookie (logout).
        final expired = cookie.expires != null && cookie.expires!.isBefore(DateTime.now());
        if (cookie.value.isEmpty || expired) {
          _cookies.remove(cookie.name);
        } else {
          _cookies[cookie.name] = cookie.value;
        }
      } on Exception {
        // A malformed Set-Cookie should not fail the request it rode in on.
        continue;
      }
    }
    _persist();
  }

  /// Dart folds repeated Set-Cookie headers into one comma-joined string, but
  /// commas also appear inside `Expires=Wed, 21 Oct 2026 ...`. Split only on
  /// commas that begin a new `name=` pair.
  static List<String> _splitSetCookie(String header) {
    final parts = <String>[];
    var start = 0;
    for (var i = 0; i < header.length; i++) {
      if (header[i] != ',') continue;
      final rest = header.substring(i + 1);
      final eq = rest.indexOf('=');
      final semi = rest.indexOf(';');
      if (eq > 0 && (semi < 0 || eq < semi) && !rest.substring(0, eq).contains(' ')) {
        parts.add(header.substring(start, i).trim());
        start = i + 1;
      }
    }
    parts.add(header.substring(start).trim());
    return parts.where((p) => p.isNotEmpty).toList();
  }

  Uri _uri(String path, [Map<String, dynamic>? query]) {
    final q = query?.entries
        .where((e) => e.value != null && '${e.value}'.isNotEmpty)
        .map((e) => MapEntry(e.key, '${e.value}'));
    return Uri.parse('$baseUrl$path').replace(
      queryParameters: q == null || q.isEmpty ? null : Map.fromEntries(q),
    );
  }

  Future<http.Response> get(String path, [Map<String, dynamic>? query]) async {
    final res = await _client.get(_uri(path, query), headers: _headers());
    _absorb(res);
    return res;
  }

  Future<http.Response> send(
    String method,
    String path, {
    Map<String, dynamic>? query,
    Map<String, String>? form,
  }) async {
    final req = http.Request(method, _uri(path, query))
      ..headers.addAll(_headers(
        unsafe: true,
        extra: form == null ? null : {'Content-Type': 'application/x-www-form-urlencoded'},
      ));
    if (form != null) req.bodyFields = form;
    final res = await http.Response.fromStream(await _client.send(req));
    _absorb(res);
    return res;
  }

  /// Multipart POST, for the endpoints that take a file rather than form
  /// fields (OPML import uses r.FormFile).
  Future<http.Response> sendFile(
    String path, {
    required String field,
    required String filename,
    required List<int> bytes,
  }) async {
    final req = http.MultipartRequest('POST', _uri(path))
      ..headers.addAll(_headers(unsafe: true))
      ..files.add(http.MultipartFile.fromBytes(field, bytes, filename: filename));
    final res = await http.Response.fromStream(await _client.send(req));
    _absorb(res);
    return res;
  }

  void close() => _client.close();
}
