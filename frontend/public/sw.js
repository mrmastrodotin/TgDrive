// Force purge all service worker caches
self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.map((k) => caches.delete(k))))
  );
  self.clients.claim();
});

// Bypass cache completely for network-first freshness
self.addEventListener('fetch', (event) => {
  // Let network handle all requests
  return;
});

