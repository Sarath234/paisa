<script lang="ts">
  import { browser } from '$app/environment';
  import { onMount } from 'svelte';
  import { toast } from 'bulma-toast';

  onMount(() => {
    if (!browser) return;
    // Dynamic import avoids Rollup resolving the virtual module during SSR
    import('virtual:pwa-register').then(({ registerSW }) => {
      registerSW({
        onOfflineReady() {
          toast({
            message: 'Paisa is ready to work offline.',
            type: 'is-success',
            dismissible: true,
            pauseOnHover: true,
            duration: 5000,
            position: 'bottom-right'
          });
        }
      });
    }).catch(() => {
      // SW not available (dev mode or non-HTTPS) — ignore
    });
  });
</script>
