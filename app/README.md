# Pulseboard app

The Flutter client for Pulseboard. Web release builds are installable PWAs:
`web/flutter_bootstrap.js` registers Pulseboard's persistent service worker,
while `web/manifest.json` contains the app identity, standalone display mode,
colors, and maskable icons. The worker deliberately does not cache
authenticated API responses or reader data.

## Run locally

```sh
flutter run -d chrome --dart-define=PULSEBOARD_BASE_URL=http://127.0.0.1:8080
```

A few resources to get you started if this is your first Flutter project:

## Build the PWA

```sh
flutter build web --release --dart-define=PULSEBOARD_BASE_URL=
```

Serve `build/web` over HTTPS (localhost also works for development). Do not
open `index.html` directly: installation and offline caching require a secure
HTTP origin. The production Nim server exposes the PWA shell files publicly so
the browser can refresh an installed app after its login session expires; API
and user data routes remain authenticated.
