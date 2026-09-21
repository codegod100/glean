import 'package:flutter/material.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'articles_screen.dart';

/// A reader's profile and, when it is your own, the settings the server keeps
/// per-user: language filters, expanded list view, and the digest toggle.
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key, this.did});

  /// Defaults to the signed-in user.
  final String? did;

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  Future<ProfileResponse>? _future;

  /// Locally applied settings, so a toggle reflects immediately instead of
  /// waiting on a full profile refetch.
  bool? _expandedView;
  bool? _digestEnabled;
  Set<String>? _languages;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    final app = AppScope.read(context);
    final did = widget.did ?? app.user?.did ?? '';
    setState(() {
      _expandedView = null;
      _digestEnabled = null;
      _languages = null;
      _future = app.client.profile(did);
    });
  }

  bool get _isSelf {
    final me = AppScope.read(context).user?.did;
    return me != null && (widget.did == null || widget.did == me);
  }

  Future<void> _setExpanded(bool v) async {
    setState(() => _expandedView = v);
    try {
      final applied = await AppScope.read(context).client.setExpandedView(v);
      if (mounted) setState(() => _expandedView = applied);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _expandedView = !v);
      showToast(context, e.message);
    }
  }

  Future<void> _setDigest(bool v) async {
    setState(() => _digestEnabled = v);
    try {
      final applied = await AppScope.read(context).client.setDigestEnabled(v);
      if (mounted) setState(() => _digestEnabled = applied);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _digestEnabled = !v);
      showToast(context, e.message);
    }
  }

  Future<void> _toggleLanguage(String code) async {
    try {
      final langs = await AppScope.read(context).client.toggleLanguage(code);
      if (mounted) setState(() => _languages = langs.toSet());
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AsyncView<ProfileResponse>(
      future: _future,
      onRetry: _load,
      builder: (context, data) => _body(context, data),
    );
  }

  Widget _body(BuildContext context, ProfileResponse data) {
    final text = Theme.of(context).textTheme;
    final u = data.profileUser;
    final expanded = _expandedView ?? data.expandedView;
    final digest = _digestEnabled ?? data.digestEnabled;
    final langs = _languages ?? data.userLanguages.toSet();

    return RefreshIndicator(
      onRefresh: () async => _load(),
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Row(
            children: [
              _Avatar(url: u.avatarUrl, seed: u.label),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(u.label, style: text.headlineSmall),
                    Text('@${u.handle}', style: text.bodySmall),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(child: _Stat(value: '${data.subscriptionCount}', label: 'feeds')),
              const SizedBox(width: 12),
              Expanded(child: _Stat(value: '${data.annotationCount}', label: 'notes')),
            ],
          ),
          if (_isSelf) ...[
            const SizedBox(height: 28),
            Text('Settings', style: text.titleMedium),
            const SizedBox(height: 8),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: expanded,
              onChanged: _setExpanded,
              title: Text('Expanded article list', style: text.bodyMedium),
              subtitle: Text('Show summaries in lists', style: text.bodySmall),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: digest,
              onChanged: AppScope.of(context).hasLlm ? _setDigest : null,
              title: Text('Daily digest', style: text.bodyMedium),
              subtitle: Text(
                AppScope.of(context).hasLlm
                    ? 'Summarise unread articles'
                    : 'Unavailable: this server has no LLM configured',
                style: text.bodySmall,
              ),
            ),
            if (data.availableLanguages.isNotEmpty) ...[
              const SizedBox(height: 20),
              Text('Languages', style: text.titleMedium),
              const SizedBox(height: 4),
              Text(
                langs.isEmpty
                    ? 'No filter: articles in any language are shown.'
                    : 'Only these languages are shown.',
                style: text.bodySmall,
              ),
              const SizedBox(height: 10),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final l in data.availableLanguages)
                    GestureDetector(
                      onTap: () => _toggleLanguage(l.code),
                      child: GleanTag(l.name, emphasis: langs.contains(l.code)),
                    ),
                ],
              ),
            ],
          ],
          if (data.subscriptions.isNotEmpty) ...[
            const SizedBox(height: 28),
            Text('Feeds', style: text.titleMedium),
            const SizedBox(height: 8),
            for (final s in data.subscriptions)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: FaviconBadge(url: s.faviconUrl, seed: s.feedTitle),
                title: Text(s.feedTitle,
                    maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodyMedium),
                onTap: () => Navigator.of(context).push(MaterialPageRoute(
                  builder: (_) => ArticlesScreen(feedUrl: s.feedUrl, title: s.feedTitle),
                )),
              ),
          ],
          const SizedBox(height: 40),
        ],
      ),
    );
  }
}

class _Avatar extends StatelessWidget {
  const _Avatar({required this.url, required this.seed});

  final String url;
  final String seed;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return Container(
      width: 56,
      height: 56,
      decoration: BoxDecoration(border: Border.all(color: c.border, width: 2)),
      child: FaviconBadge(url: url, seed: seed, size: 52),
    );
  }
}

class _Stat extends StatelessWidget {
  const _Stat({required this.value, required this.label});

  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return GleanBox(
      filled: true,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(value, style: text.displaySmall),
          Text(label, style: text.bodySmall),
        ],
      ),
    );
  }
}
