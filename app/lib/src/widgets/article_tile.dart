import 'package:flutter/material.dart';

import '../models/models.dart';
import '../theme.dart';
import 'common.dart';

/// One row in the article list. Read state shows as weight and opacity
/// rather than a badge, so an unread item simply reads louder.
class ArticleTile extends StatelessWidget {
  const ArticleTile({
    super.key,
    required this.article,
    required this.onTap,
    this.onToggleRead,
  });

  final Article article;
  final VoidCallback onTap;
  final VoidCallback? onToggleRead;

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
                Expanded(
                  child: Text(article.feedTitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: text.bodySmall),
                ),
                if (article.published != null)
                  Text(relativeTime(article.published), style: text.bodySmall),
              ],
            ),
            const SizedBox(height: 6),
            Opacity(
              opacity: read ? 0.55 : 1,
              child: Text(
                article.title.isEmpty ? '(untitled)' : article.title,
                maxLines: 3,
                overflow: TextOverflow.ellipsis,
                style: text.bodyLarge?.copyWith(
                  fontWeight: read ? FontWeight.w400 : FontWeight.w700,
                ),
              ),
            ),
            if (article.summary.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                // Feed summaries are HTML; show them as the text they read as.
                stripHtml(article.summary),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: text.bodySmall,
              ),
            ],
            if (onToggleRead != null)
              Align(
                alignment: Alignment.centerRight,
                child: IconButton(
                  visualDensity: VisualDensity.compact,
                  tooltip: read ? 'Mark unread' : 'Mark read',
                  icon: Icon(
                    read ? Icons.mark_email_unread_outlined : Icons.check,
                    size: 18,
                    color: c.muted,
                  ),
                  onPressed: onToggleRead,
                ),
              ),
          ],
        ),
      ),
    );
  }
}
