<script lang="ts">
  import { afterNavigate, beforeNavigate } from "$app/navigation";
  import { page } from "$app/stores";
  import { onMount } from "svelte";
  import { followCursor, delegate, hideAll } from "tippy.js";
  import _ from "lodash";
  import Spinner from "$lib/components/Spinner.svelte";
  import Navbar from "$lib/components/Navbar.svelte";
  import QuickEntryModal from "$lib/components/QuickEntryModal.svelte";
  import SearchModal from "$lib/components/SearchModal.svelte";
  import ChatWidget from "$lib/components/ChatWidget.svelte";
  import { willClearTippy, willRefresh } from "../../store";

  let quickEntryActive = false;
  let searchActive = false;

  let isBurger: boolean = null;

  function clearTippy() {
    hideAll();
  }

  function setupTippy() {
    delegate("body", {
      target: "[data-tippy-content]",
      theme: "light",
      onShow: (instance) => {
        const content = instance.reference.getAttribute("data-tippy-content");
        if (!_.isEmpty(content)) {
          instance.setContent(content);
        } else {
          return false;
        }
      },
      maxWidth: "none",
      delay: 0,
      allowHTML: true,
      followCursor: true,
      popperOptions: {
        modifiers: [
          {
            name: "flip",
            options: {
              fallbackPlacements: ["auto"]
            }
          }
        ]
      },
      plugins: [followCursor]
    });
  }

  willClearTippy.subscribe(clearTippy);
  beforeNavigate(clearTippy);
  willRefresh.subscribe(() => {
    clearTippy();
    setupTippy();
  });

  afterNavigate(() => {
    isBurger = null;
    setupTippy();
  });

  onMount(() => {
    function handleKeydown(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement)?.tagName?.toLowerCase();
      const isEditable =
        tag === "input" ||
        tag === "textarea" ||
        tag === "select" ||
        (e.target as HTMLElement)?.isContentEditable;
      if ((e.key === "n" || e.key === "N") && !isEditable) {
        quickEntryActive = true;
      }
      if (e.key === "/" && !isEditable) {
        e.preventDefault();
        searchActive = true;
      }
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  });
</script>

<QuickEntryModal bind:active={quickEntryActive} />
<SearchModal bind:active={searchActive} />
{#if $page.url.pathname !== "/assistant"}
  <ChatWidget />
{/if}

{#key $willRefresh}
  <Navbar
    bind:isBurger
    on:quickentry={() => (quickEntryActive = true)}
    on:search={() => (searchActive = true)}
  />

  <Spinner>
    <slot />
  </Spinner>
{/key}
