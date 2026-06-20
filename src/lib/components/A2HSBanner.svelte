<script lang="ts">
  import { onMount } from 'svelte';

  const STORAGE_KEY = 'a2hs_dismissed';

  let show = false;
  let isIOS = false;
  let deferredPrompt: any = null;

  onMount(() => {
    if (localStorage.getItem(STORAGE_KEY)) return;

    const ua = navigator.userAgent;
    isIOS = /iphone|ipad|ipod/i.test(ua) && !(window as any).MSStream;

    if (isIOS) {
      // Show static iOS instructions (no beforeinstallprompt on Safari)
      const isStandalone = (window.navigator as any).standalone;
      if (!isStandalone) {
        show = true;
      }
      return;
    }

    window.addEventListener('beforeinstallprompt', (e: Event) => {
      e.preventDefault();
      deferredPrompt = e;
      show = true;
    });
  });

  async function install() {
    if (deferredPrompt) {
      deferredPrompt.prompt();
      const { outcome } = await deferredPrompt.userChoice;
      deferredPrompt = null;
      if (outcome === 'accepted') {
        dismiss();
      }
    }
  }

  function dismiss() {
    localStorage.setItem(STORAGE_KEY, '1');
    show = false;
  }
</script>

{#if show}
  <div class="notification is-info is-light a2hs-banner">
    <button class="delete" on:click={dismiss} aria-label="Dismiss"></button>
    {#if isIOS}
      <span>
        <strong>Add to Home Screen:</strong> tap the Share button
        <i class="fas fa-arrow-up-from-bracket" /> then <em>Add to Home Screen</em>.
      </span>
    {:else}
      <span>Add <strong>Paisa</strong> to your home screen for quick access.</span>
      <button class="button is-small is-info ml-2" on:click={install}>Install</button>
    {/if}
  </div>
{/if}

<style>
  .a2hs-banner {
    position: fixed;
    bottom: 1rem;
    left: 1rem;
    right: 1rem;
    z-index: 9999;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.15);
  }
</style>
