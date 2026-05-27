<script lang="ts">
  import InsightCard from "$lib/components/InsightCard.svelte";
  import { ajax, type Insight } from "$lib/utils";
  import _ from "lodash";
  import { onMount } from "svelte";

  let insights: Insight[] = [];
  let loaded = false;

  onMount(async () => {
    ({ insights } = await ajax("/api/insights"));
    insights = _.orderBy(
      insights.filter((i) => !i.suppress),
      (i) => Math.abs(i.delta_pct),
      "desc"
    );
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    <h1 class="title">Insights</h1>
    {#if loaded && insights.length === 0}
      <p class="has-text-grey">No insights yet — add some transactions first.</p>
    {:else}
      <div class="columns is-multiline">
        {#each insights as insight}
          <div class="column is-4">
            <InsightCard {insight} />
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>
