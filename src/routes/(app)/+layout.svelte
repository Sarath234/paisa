<script lang="ts">
  import { afterNavigate, beforeNavigate } from "$app/navigation";
  import { onMount } from "svelte";
  import { followCursor, delegate, hideAll } from "tippy.js";
  import _ from "lodash";
  import Spinner from "$lib/components/Spinner.svelte";
  import Navbar from "$lib/components/Navbar.svelte";
  import QuickEntryModal from "$lib/components/QuickEntryModal.svelte";
  import { willClearTippy, willRefresh } from "../../store";

  let quickEntryActive = false;

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
      if (e.key === "n" || e.key === "N") {
        const tag = (e.target as HTMLElement)?.tagName?.toLowerCase();
        if (tag === "input" || tag === "textarea" || tag === "select") return;
        if ((e.target as HTMLElement)?.isContentEditable) return;
        quickEntryActive = true;
      }
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  });
</script>

<QuickEntryModal bind:active={quickEntryActive} />

{#key $willRefresh}
  <Navbar bind:isBurger on:quickentry={() => (quickEntryActive = true)} />

  <Spinner>
    <slot />
  </Spinner>
{/key}
