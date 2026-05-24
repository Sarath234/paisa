<script lang="ts">
  import { page } from "$app/stores";
  import { ajax, type Transaction } from "$lib/utils";
  import TransactionCard from "$lib/components/TransactionCard.svelte";
  import { MasonryGrid } from "@egjs/svelte-grid";
  import _ from "lodash";
  import { onMount } from "svelte";

  let UntypedMasonryGrid = MasonryGrid as any;
  let transactions: Transaction[] = [];
  let loaded = false;

  $: q = $page.url.searchParams.get("q") ?? "";

  $: filtered = (() => {
    if (!q.trim()) return [];
    const lower = q.toLowerCase();
    return transactions.filter(
      (t) =>
        t.payee.toLowerCase().includes(lower) ||
        t.postings.some(
          (p) =>
            p.account.toLowerCase().includes(lower) || (p.note ?? "").toLowerCase().includes(lower)
        )
    );
  })();

  onMount(async () => {
    ({ transactions } = await ajax("/api/transaction"));
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    {#if !q.trim()}
      <p class="has-text-grey">Type to search transactions by payee, account, or note.</p>
    {:else if !loaded}
      <p class="has-text-grey">Loading…</p>
    {:else if filtered.length === 0}
      <p class="has-text-grey">No transactions found for <strong>{q}</strong>.</p>
    {:else}
      <p class="mb-3 has-text-grey is-size-7">
        <strong>{filtered.length}</strong> transaction(s) found
      </p>
      {#key q}
        <UntypedMasonryGrid gap={10} maxStretchColumnSize={500} align="stretch">
          {#each filtered as t (t.id)}
            <div class="mr-3 is-flex-grow-1">
              <TransactionCard {t} />
            </div>
          {/each}
        </UntypedMasonryGrid>
      {/key}
    {/if}
  </div>
</section>
