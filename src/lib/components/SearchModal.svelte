<script lang="ts">
  import Modal from "$lib/components/Modal.svelte";
  import TransactionCard from "$lib/components/TransactionCard.svelte";
  import { ajax, type Transaction } from "$lib/utils";
  import { onMount, tick } from "svelte";

  export let active = false;

  let query = "";
  let transactions: Transaction[] = [];
  let loaded = false;
  let inputEl: HTMLInputElement;

  $: filtered = (() => {
    if (!query.trim()) return [];
    const lower = query.toLowerCase();
    return transactions.filter(
      (t) =>
        t.payee.toLowerCase().includes(lower) ||
        t.postings.some(
          (p) =>
            p.account.toLowerCase().includes(lower) || (p.note ?? "").toLowerCase().includes(lower)
        )
    );
  })();

  $: if (active) {
    tick().then(() => inputEl?.focus());
    if (!loaded) {
      ajax("/api/transaction").then(({ transactions: txs }) => {
        transactions = txs;
        loaded = true;
      });
    }
  } else {
    query = "";
  }
</script>

<Modal bind:active width="min(900px, 100vw)" bodyClass="p-4">
  <svelte:fragment slot="head">
    <div class="field mb-0" style="flex: 1">
      <div class="control has-icons-left">
        <input
          bind:this={inputEl}
          class="input"
          type="search"
          placeholder="Search transactions by payee, account, or note…"
          bind:value={query}
        />
        <span class="icon is-left"><i class="fas fa-search" /></span>
      </div>
    </div>
  </svelte:fragment>

  <svelte:fragment slot="body">
    {#if !query.trim()}
      <p class="has-text-grey">Type to search transactions by payee, account, or note.</p>
    {:else if !loaded}
      <p class="has-text-grey">Loading…</p>
    {:else if filtered.length === 0}
      <p class="has-text-grey">No transactions found for <strong>{query}</strong>.</p>
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
  </svelte:fragment>

  <svelte:fragment slot="foot" let:close>
    <button class="button" on:click={close}>Close</button>
  </svelte:fragment>
</Modal>

<style>
  .search-results {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }

  .search-results :global(.box) {
    flex: 1 1 300px;
    max-width: 480px;
  }
</style>
