// Service Worker для PWA
const CACHE_NAME = 'Business Stack-v5';
const urlsToCache = [
  '/',
  '/dashboard-improved',
  '/inventory',
  '/suppliers',
  '/finance',
  '/reports'
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        console.log('Opened cache');
        // Кэшируем каждый URL отдельно, чтобы ошибки не блокировали остальные
        return Promise.allSettled(
          urlsToCache.map(url => {
            return fetch(url)
              .then(response => {
                if (response.ok) {
                  return cache.put(url, response);
                }
                console.log('Skip caching:', url, 'status:', response.status);
                return Promise.resolve();
              })
              .catch(err => {
                console.log('Failed to cache:', url, err);
                return Promise.resolve();
              });
          })
        );
      })
  );
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(cacheNames => {
      return Promise.all(
        cacheNames.map(cacheName => {
          if (cacheName !== CACHE_NAME) {
            console.log('Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', event => {
  // Для API запросов - только сеть
  if (event.request.url.includes('/api/')) {
    event.respondWith(fetch(event.request));
    return;
  }
  
  event.respondWith(
    caches.match(event.request)
      .then(response => {
        if (response) {
          return response;
        }
        return fetch(event.request);
      })
      .catch(() => {
        // Офлайн: показываем главную страницу
        return caches.match('/');
      })
  );
});
