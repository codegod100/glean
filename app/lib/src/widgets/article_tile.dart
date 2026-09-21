import 'package:flutter/material.dart';

import '../models/models.dart';
import '../theme.dart';
import 'common.dart';

/// One row in any article list.
///
/// Read state is carried by weight and opacity rather than a separate badge,
/// matching the web list where unread items simply read louder.
class ArticleTile extends StatelessWidget {
  const ArticleTile({
    super.key,
    required this.article,
    required this.onTap,
    this.onToggleRead,
    this.onToggleLike,
    this.expanded = false,
  });

  final Article article;
  final VoidCallback onTap;
  final VoidCallback? onToggleRead;
  final VoidCallback? onToggleLike;

  /// Mirrors the server's expanded_view preference: show the summary too.
  final bool expanded;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    final text = Theme.of(context).textTheme;
    final read = article.isRead;

    return InkWell(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          border: Border(bottom: BorderSide(color: c.faint, width: 1)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                FaviconBadge(url: article.feedFaviconUrl, seed: article.feedTitle),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    article.feedTitle,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: text.bodySmall,
                  ),
                ),
                if (article.published != null)
                  Text(relativeTime(article.published), style: text.bodySmall),
              ],
            ),
            const SizedBox(height: 6),
            Opacity(
              opacity: read ? 0.55 : 1,
              child: Text(
                article.title,
                maxLines: 3,
                overflow: TextOverflow.ellipsis,
                style: text.bodyLarge?.copyWith(
                  fontWeight: read ? FontWeight.w400 : FontWeight.w700,
                ),
              ),
            ),
            if (expanded && article.summary.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                article.summary,
                maxLines: 3,
                overflow: TextOverflow.ellipsis,
                style: text.bodySmall,
              ),
            ],
            const SizedBox(height: 8),
            Row(
              children: [
                if (article.author.isNotEmpty) ...[
                  Flexible(
                    child: Text(
                      article.author,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: text.bodySmall,
                    ),
                  ),
                  const SizedBox(width: 12),
                ],
                const Spacer(),
                if (onToggleLike != null)
                  _IconCount(
                    icon: article.hasLiked ? Icons.favorite : Icons.favorite_border,
                    count: article.likeCount,
                    active: article.hasLiked,
                    onTap: onToggleLike!,
                  ),
                if (onToggleRead != null)
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    tooltip: read ? 'Mark unread' : 'Mark read',
                    icon: Icon(
                      read ? Icons.mark_email_unread_outlined : Icons.check,
                      size: 18,
                      color: c.muted,
                    ),
                    onPressed: onToggleRead,
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _IconCount extends StatelessWidget {
  const _IconCount({
    required this.icon,
    required this.count,
    required this.active,
    required this.onTap,
  });

  final IconData icon;
  final int count;
  final bool active;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 16, color: active ? c.danger : c.muted),
            if (count > 0) ...[
              const SizedBox(width: 4),
              Text('$count', style: Theme.of(context).textTheme.bodySmall),
            ],
          ],
        ),
      ),
    );
  }
}
