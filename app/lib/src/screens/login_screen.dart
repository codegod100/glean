import 'package:flutter/material.dart';
import 'package:webview_cookie_manager/webview_cookie_manager.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../api/client.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// Sign-in via ATProto OAuth.
///
/// The Go server drives the whole flow in a browser: POST /api/auth/start
/// returns the PDS authorization URL, the user authorizes there, and the PDS
/// redirects back to /api/auth/callback, which sets the session cookie and
/// redirects on. A native app has no browser to inherit that cookie from, so
/// the flow runs in a webview and the cookie is lifted out of it once the
/// callback has landed. That keeps the server contract untouched -- the
/// alternative, a native deep-link redirect URI, would mean changing the
/// published OAuth client metadata.
class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _controller = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    final handle = _controller.text.trim().replaceFirst(RegExp(r'^@'), '');
    if (handle.isEmpty) {
      setState(() => _error = 'Enter your handle.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });

    final app = AppScope.read(context);
    try {
      final target = await app.client.startAuth(handle);
      if (!mounted) return;

      // The server answers with /dashboard when it fell back to creating a
      // local account without OAuth; there is nothing to authorize.
      if (!target.startsWith('http')) {
        await app.refreshUser();
        return;
      }

      final cookies = await Navigator.of(context).push<Map<String, String>>(
        MaterialPageRoute(
          builder: (_) => _OAuthWebView(authUrl: target, baseUrl: app.baseUrl),
        ),
      );
      if (!mounted) return;
      if (cookies == null || cookies.isEmpty) {
        setState(() => _error = 'Sign-in was cancelled.');
        return;
      }
      await app.completeSignIn(cookies);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } on Exception {
      if (mounted) setState(() => _error = 'Could not reach the server.');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final c = GleanColors.of(context);
    final text = Theme.of(context).textTheme;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text('glean', style: text.displaySmall),
                  const SizedBox(height: 4),
                  Text('a reader for the open social web', style: text.bodySmall),
                  const SizedBox(height: 28),
                  if (!app.oauthConfigured)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 16),
                      child: GleanBox(
                        filled: true,
                        child: Text(
                          'This server has no OAuth client configured '
                          '(GLEAN_OAUTH_CLIENT_ID), so sign-in cannot complete. '
                          'Browse trending in the meantime.',
                          style: text.bodySmall,
                        ),
                      ),
                    ),
                  TextField(
                    controller: _controller,
                    autocorrect: false,
                    enableSuggestions: false,
                    keyboardType: TextInputType.url,
                    decoration: const InputDecoration(hintText: 'you.bsky.social'),
                    onSubmitted: (_) => _start(),
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: 10),
                    Text(_error!, style: text.bodySmall?.copyWith(color: c.danger)),
                  ],
                  const SizedBox(height: 16),
                  GleanButton(
                    label: 'Sign in',
                    accent: true,
                    busy: _busy,
                    onPressed: app.oauthConfigured ? _start : null,
                  ),
                  const SizedBox(height: 12),
                  Center(
                    child: TextButton(
                      onPressed: () => Navigator.of(context).maybePop(),
                      child: Text('Keep browsing', style: text.bodySmall),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Hosts the PDS authorization page and pops the session cookies once the
/// server's callback has completed.
class _OAuthWebView extends StatefulWidget {
  const _OAuthWebView({required this.authUrl, required this.baseUrl});

  final String authUrl;
  final String baseUrl;

  @override
  State<_OAuthWebView> createState() => _OAuthWebViewState();
}

class _OAuthWebViewState extends State<_OAuthWebView> {
  late final WebViewController _controller;
  bool _finishing = false;

  @override
  void initState() {
    super.initState();
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(
        NavigationDelegate(onPageFinished: _onPageFinished),
      )
      ..loadRequest(Uri.parse(widget.authUrl));
  }

  Future<void> _onPageFinished(String url) async {
    if (_finishing) return;
    final uri = Uri.tryParse(url);
    final base = Uri.tryParse(widget.baseUrl);
    if (uri == null || base == null || uri.host != base.host) return;

    // The callback redirects onward once the cookie is set, so treat any
    // same-origin page that is not the login screen as a completed flow.
    if (uri.path.startsWith('/auth/login')) {
      if (uri.queryParameters['error'] != null && mounted) {
        Navigator.of(context).pop(<String, String>{});
      }
      return;
    }

    _finishing = true;
    final cookies = await _readCookies();
    if (!mounted) return;
    Navigator.of(context).pop(cookies);
  }

  /// Read the cookies out of the platform cookie store rather than via
  /// document.cookie: the session cookie is HttpOnly (internal/server/
  /// session.go), so JavaScript can only ever see the CSRF one, and adopting
  /// that alone would leave the app looking signed in but unauthenticated.
  Future<Map<String, String>> _readCookies() async {
    final jar = await WebviewCookieManager().getCookies(widget.baseUrl);
    return {for (final c in jar) c.name: c.value};
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Authorize')),
      body: WebViewWidget(controller: _controller),
    );
  }
}
