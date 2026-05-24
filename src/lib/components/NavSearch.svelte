<script lang="ts">
  import { createEventDispatcher } from "svelte";
  const dispatch = createEventDispatcher();
</script>

<button
  class="button is-small is-rounded"
  data-tippy-content="<p>Search transactions (/)</p>"
  on:click={() => dispatch("search")}
>
  <span class="icon is-small"><i class="fas fa-search" /></span>
</button>
