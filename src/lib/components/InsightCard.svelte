<!-- src/lib/components/InsightCard.svelte -->
<script lang="ts">
  import type { Insight } from "$lib/utils";

  export let insight: Insight;

  const icons: Record<string, string> = {
    spend_category: "💰",
    spend_category_weekly: "💰",
    savings_rate: "📈",
    budget: "🏷",
    top_category: "🏆",
    top_category_weekly: "🏆",
    income: "💼"
  };

  $: icon = icons[insight.type] ?? "💡";
  $: showBadge = !insight.type.startsWith("top_category");
  $: badgeClass = insight.positive ? "has-text-success" : "has-text-danger";
  $: unit = insight.type === "savings_rate" ? " pp" : "%";
  $: badgeText =
    insight.delta_pct > 0
      ? `+${Math.abs(insight.delta_pct).toFixed(0)}${unit}`
      : `${insight.delta_pct.toFixed(0)}${unit}`;
</script>

<div class="box p-4">
  <div class="is-flex is-align-items-center mb-2" style="gap: 0.5rem">
    <span style="font-size: 1.25rem">{icon}</span>
    <strong>{insight.title}</strong>
    {#if showBadge}
      <span class="tag {badgeClass} ml-auto">{badgeText}</span>
    {/if}
  </div>
  <p class="is-size-7 has-text-grey-dark">{insight.body}</p>
</div>
