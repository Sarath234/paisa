<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/stores";
  import _ from "lodash";

  let value = "";

  // Keep input in sync when navigating with browser back/forward
  $: {
    const q = $page.url.searchParams.get("q") ?? "";
    if (q !== value) value = q;
  }

  const debouncedNavigate = _.debounce((q: string) => {
    const url = q ? `/search?q=${encodeURIComponent(q)}` : "/search";
    goto(url, { replaceState: true, keepFocus: true });
  }, 300);

  function handleInput(e: Event) {
    value = (e.target as HTMLInputElement).value;
    debouncedNavigate(value);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      value = "";
      debouncedNavigate.cancel();
      goto("/search", { replaceState: true, keepFocus: true });
    }
  }
</script>

<input
  class="input is-small"
  type="search"
  placeholder="Search transactions…"
  style="width: 200px"
  {value}
  on:input={handleInput}
  on:keydown={handleKeydown}
/>
