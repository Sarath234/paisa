/// <reference lib="webworker" />
import { precacheAndRoute, cleanupOutdatedCaches } from "workbox-precaching";
import { registerRoute } from "workbox-routing";
import { NetworkFirst } from "workbox-strategies";
import { CacheableResponsePlugin } from "workbox-cacheable-response";
import { ExpirationPlugin } from "workbox-expiration";

declare const self: ServiceWorkerGlobalScope;

precacheAndRoute(self.__WB_MANIFEST);
cleanupOutdatedCaches();

registerRoute(
  ({ url }) => url.pathname.startsWith("/api/"),
  new NetworkFirst({
    networkTimeoutSeconds: 3,
    cacheName: "api-cache",
    plugins: [
      new CacheableResponsePlugin({ statuses: [200] }),
      new ExpirationPlugin({ maxEntries: 50, maxAgeSeconds: 86400 })
    ]
  })
);

self.addEventListener("push", (event) => {
  const data = event.data?.json() ?? { title: "Paisa", body: "" };
  event.waitUntil(
    self.registration.showNotification(data.title as string, {
      body: data.body as string,
      icon: "/pwa-assets/icon-192.png",
      badge: "/pwa-assets/icon-192.png"
    })
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(
    (self as unknown as { clients: Clients }).clients.matchAll({ type: "window" }).then((clientList) => {
      for (const client of clientList) {
        if ("focus" in client) return (client as WindowClient).focus();
      }
      if ((self as unknown as { clients: Clients }).clients.openWindow) {
        return (self as unknown as { clients: Clients }).clients.openWindow("/");
      }
    })
  );
});
