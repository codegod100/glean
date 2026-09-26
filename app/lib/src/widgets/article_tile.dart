import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../models/models.dart';
import '../theme.dart';
import 'common.dart';

/// One row in the article list. An editorial title and excerpt are paired with
/// quiet, technical metadata to make the list scan like a compact paper shelf.
class ArticleTile extends StatelessWidget {
  const ArticleTile({
    super.key,
    required this.article,
    required this.uri,
    required this.onTap,
    this.onToggleRead,
  });

  final Article article;
  final Uri? uri;
  final VoidCallback onTap;
  final VoidCallback? onToggleRead;

  @override
  Widget build(BuildContext context) {
    final c = PulseboardColors.of(context);
    final text = Theme.of(context).textTheme;
    final read = article.isRead;

    return Semantics(
      link: uri != null,
      child: InkWell(
        onTap: () {
          onTap();
          if (uri != null) {
            // Avoid url_launcher's Link widget here. On the web it is backed by
            // a focusable platform view; Flutter can hide that view while its
            // anchor still owns focus, which makes the browser report an
            // aria-hidden accessibility violation. Launching directly has no
            // platform view, while `_blank` still sends links outside an
            // installed PWA.
            launchUrl(uri!, webOnlyWindowName: '_blank');
          }
        },
        child: Container(
          padding: const EdgeInsets.fromLTRB(20, 18, 10, 18),
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
                            style: text.bodySmall?.copyWith(
                              letterSpacing: 0.25,
                            ),
                          ),
                        ),
                        if (article.published != null) ...[
                          const SizedBox(width: 12),
                          Text(
                            relativeTime(article.published),
                            style: text.bodySmall?.copyWith(
                              letterSpacing: 0.25,
                            ),
                          ),
                        ],
                      ],
                    ),
                    const SizedBox(height: 10),
                    Opacity(
                      opacity: read ? 0.55 : 1,
                      child: Text(
                        article.title.isEmpty ? '(untitled)' : article.title,
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                        style: text.bodyLarge?.copyWith(
                          fontWeight: read ? FontWeight.w400 : FontWeight.w600,
                          height: 1.35,
                        ),
                      ),
                    ),
                    if (article.summary.isNotEmpty) ...[
                      const SizedBox(height: 9),
                      Text(
                        stripHtml(article.summary),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: text.bodyMedium?.copyWith(
                          color: read ? c.muted.withOpacity(0.78) : c.muted,
                          height: 1.5,
                        ),
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
      ),
    );
  }
}
