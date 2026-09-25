{{flutter_js}}
{{flutter_build_config}}

// Flutter no longer owns a caching service worker. Register Pulseboard's
// persistent worker before starting the application instead of accepting the
// generated worker that unregisters itself.
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('pulseboard_service_worker.js').catch(
    (error) => console.error('Pulseboard service worker registration failed:', error),
  );
}

_flutter.loader.load();
