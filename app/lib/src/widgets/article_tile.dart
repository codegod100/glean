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
    final c = PulseboardColors.of(context);
    final text = Theme.of(context).textTheme;
    final read = article.isRead;

    return InkWell(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.fromLTRB(16, 14, 8, 14),
        decoration: BoxDecoration(
          border: Border(bottom: BorderSide(color: c.faint, width: 1)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Container(
                        width: 7,
                        height: 7,
                        decoration: BoxDecoration(
                          color: read ? c.faint : c.accent,
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 7),
                      Expanded(
                        child: Text(
                          article.feedTitle,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: text.bodySmall,
                        ),
                      ),
                      if (article.published != null) ...[
                        const SizedBox(width: 12),
                        Text(
                          relativeTime(article.published),
                          style: text.bodySmall,
                        ),
                      ],
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
                      stripHtml(article.summary),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: text.bodySmall,
                    ),
                  ],
                ],
              ),
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
      ),
    );
  }
}
