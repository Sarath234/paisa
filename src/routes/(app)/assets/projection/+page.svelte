<script lang="ts">
  import { ajax, formatCurrency, isMobile, type Legend } from "$lib/utils";
  import COLORS from "$lib/colors";
  import { renderProjection } from "$lib/projection";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";
  import LevelItem from "$lib/components/LevelItem.svelte";
  import ZeroState from "$lib/components/ZeroState.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";

  const HORIZONS = [1, 3, 5, 10];

  let currentNetworth = 0;
  let currentInvestment = 0;
  let monthlySavings = 0;
  let horizonYears = 5;
  let returnPct = 12;
  let loaded = false;

  let svg: Element;
  let destroy: (() => void) | null = null;
  let legends: Legend[] = [];

  function projectedValue(): number {
    const r = returnPct / 100 / 12;
    let w = currentNetworth;
    for (let i = 0; i < horizonYears * 12; i++) {
      w = w * (1 + r) + monthlySavings;
    }
    return w;
  }

  $: if (loaded && svg) {
    if (destroy) destroy();
    ({ destroy, legends } = renderProjection(
      currentNetworth,
      currentInvestment,
      monthlySavings,
      horizonYears,
      returnPct,
      svg
    ));
  }

  onDestroy(() => {
    if (destroy) destroy();
  });

  onMount(async () => {
    const result = await ajax("/api/networth");
    const last = _.last(result.networthTimeline);
    if (last) {
      currentNetworth = last.investmentAmount + last.gainAmount - last.withdrawalAmount;
      currentInvestment = last.investmentAmount - last.withdrawalAmount;
    }
    monthlySavings = result.monthlySavings;
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    <nav class="level {isMobile() && 'grid-2'}">
      <LevelItem
        title="Current Net Worth"
        color={COLORS.primary}
        value={formatCurrency(currentNetworth)}
      />
      <LevelItem
        title="Monthly Savings"
        color={COLORS.secondary}
        value={formatCurrency(monthlySavings)}
        subtitle="12-month avg"
      />
      <LevelItem
        title="Projected in {horizonYears}Y"
        color={COLORS.gainText}
        value={loaded ? formatCurrency(projectedValue()) : "—"}
        subtitle="at {returnPct}% return"
      />
    </nav>
  </div>
</section>

<section class="section">
  <div class="container is-fluid">
    <div class="box p-4 mb-4">
      <div class="columns is-vcentered is-mobile">
        <div class="column is-narrow">
          <span class="label is-small mb-0">Horizon</span>
          <div class="buttons has-addons mt-1">
            {#each HORIZONS as h}
              <button
                class="button is-small {horizonYears === h ? 'is-primary' : ''}"
                on:click={() => (horizonYears = h)}>{h}Y</button
              >
            {/each}
          </div>
        </div>
        <div class="column">
          <span class="label is-small mb-0">Annual Return: {returnPct}%</span>
          <input
            class="mt-1"
            type="range"
            min="6"
            max="18"
            step="1"
            bind:value={returnPct}
            style="width:100%;accent-color:{COLORS.primary}"
          />
        </div>
      </div>
    </div>

    <div class="columns">
      <div class="column is-12">
        <div class="box overflow-x-auto">
          <ZeroState item={loaded ? [currentNetworth] : []}>
            <strong>Oops!</strong> You have no transactions.
          </ZeroState>
          <LegendCard {legends} clazz="ml-4" />
          <svg id="d3-projection" height="500" bind:this={svg} />
        </div>
      </div>
    </div>
  </div>
</section>
