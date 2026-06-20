<script lang="ts">
  import { onMount } from 'svelte';

  const STORAGE_KEY = 'push_banner_dismissed';

  let show = false;

  onMount(() => {
    if (localStorage.getItem(STORAGE_KEY)) return;
    if (!('Notification' in window) || !('serviceWorker' in navigator)) return;
    if (Notification.permission !== 'default') return;

    // Show after 30 seconds of active session
    const timer = setTimeout(() => { show = true; }, 30_000);
    return () => clearTimeout(timer);
  });

  async function requestPermission() {
    const permission = await Notification.requestPermission();

    if (permission !== 'granted') {
      dismiss();
      return;
    }

    try {
      const reg = await navigator.serviceWorker.ready;
      const res = await fetch('/api/push/public-key');
      const { publicKey } = await res.json();
      if (!publicKey) { dismiss(); return; }

      const subscription = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(publicKey)
      });

      const subRes = await fetch('/api/push/subscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(subscription.toJSON())
      });
      if (!subRes.ok) throw new Error(`subscribe failed: ${subRes.status}`);

      dismiss();
    } catch (err) {
      console.warn('push subscribe failed:', err);
      dismiss();
    }
  }

  function dismiss() {
    localStorage.setItem(STORAGE_KEY, '1');
    show = false;
  }

  function urlBase64ToUint8Array(base64String: string): Uint8Array {
    const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
    const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
    const raw = window.atob(base64);
    return Uint8Array.from([...raw].map((c) => c.charCodeAt(0)));
  }
</script>

{#if show}
  <div class="notification is-warning is-light push-banner" role="alert">
    <button class="delete" on:click={dismiss} aria-label="Dismiss"></button>
    <span>Get notified when your budget is overspent.</span>
    <button class="button is-small is-warning ml-2" on:click={requestPermission}>
      Enable notifications
    </button>
  </div>
{/if}

<style>
  .push-banner {
    position: fixed;
    bottom: 5rem;
    left: 1rem;
    right: 1rem;
    z-index: 9998;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.15);
  }
</style>
