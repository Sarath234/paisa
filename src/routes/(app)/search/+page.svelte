<script lang="ts">
  import { page } from "$app/stores";
  import { ajax, type Transaction } from "$lib/utils";
  import TransactionCard from "$lib/components/TransactionCard.svelte";
  import { onMount } from "svelte";

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
      <div class="search-results">
        {#each filtered as t (t.id)}
          <TransactionCard {t} />
        {/each}
      </div>
    {/if}
  </div>
</section>

<style>
  .search-results {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }

  .search-results :global(.box) {
    flex: 1 1 300px;
    max-width: 500px;
  }
</style>
