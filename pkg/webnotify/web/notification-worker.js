// Local notifications only: no Push subscription, network proxy or offline cache.
self.addEventListener('install', event => event.waitUntil(self.skipWaiting()));
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
self.addEventListener('message', event => {
  if (event.data?.type === 'microlight-notification-client' && event.source && event.ports[0]) {
    event.ports[0].postMessage(event.source.id);
  }
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  event.waitUntil((async () => {
    const id = event.notification.data?.clientID;
    const page = id ? await self.clients.get(id) : null;
    if (page) {
      await page.focus();
      page.postMessage({type:'microlight-notification-click',tag:event.notification.tag});
    } else {
      await self.clients.openWindow(self.registration.scope);
    }
  })());
});
