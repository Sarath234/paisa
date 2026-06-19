<script lang="ts">
  import InsightCard from "$lib/components/InsightCard.svelte";
  import { ajax, type Insight } from "$lib/utils";
  import _ from "lodash";
  import { onMount } from "svelte";

  const GROUP_LABEL: Record<string, string> = {
    budget: "Budget",
    income: "Income",
    expenses: "Expenses",
    savings_rate: "Savings Rate",
    other: "Other"
  };

  const GROUP_ORDER = ["budget", "income", "expenses", "savings_rate", "other"];

  function insightGroup(insight: Insight): string {
    if (insight.type === "budget") return "budget";
    if (insight.type === "income") return "income";
    if (
      insight.type === "spend_category" ||
      insight.type === "spend_category_weekly" ||
      insight.type === "top_category" ||
      insight.type === "top_category_weekly"
    )
      return "expenses";
    if (insight.type === "savings_rate") return "savings_rate";
    return "other";
  }

  let groups: { key: string; label: string; items: Insight[] }[] = [];
  let loaded = false;

  onMount(async () => {
    const { insights } = await ajax("/api/insights");
    const visible = insights.filter((i) => !i.suppress);
    const byGroup = _.groupBy(visible, insightGroup);

    groups = GROUP_ORDER.filter((g) => byGroup[g]?.length).map((g) => ({
      key: g,
      label: GROUP_LABEL[g],
      items: _.orderBy(byGroup[g], (i) => Math.abs(i.delta_pct), "desc")
    }));

    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    <h1 class="title">Insights</h1>
    {#if loaded && groups.length === 0}
      <p class="has-text-grey">No insights yet — add some transactions first.</p>
    {:else}
      {#each groups as group}
        <h2 class="subtitle mb-2 mt-5">{group.label}</h2>
        <div class="columns is-multiline mb-0">
          {#each group.items as insight}
            <div class="column is-4">
              <InsightCard {insight} />
            </div>
          {/each}
        </div>
      {/each}
    {/if}
  </div>
</section>
