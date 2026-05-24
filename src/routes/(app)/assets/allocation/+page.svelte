<script lang="ts">
  import {
    renderAllocation,
    renderAllocationTarget,
    renderAllocationTimeline
  } from "$lib/allocation";
  import COLORS, { generateColorScheme } from "$lib/colors";
  import BoxLabel from "$lib/components/BoxLabel.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";
  import Table from "$lib/components/Table.svelte";
  import { accountName, nonZeroCurrency } from "$lib/table_formatters";
  import {
    ajax,
    formatCurrency,
    formatPercentage,
    rem,
    type Aggregate,
    type AllocationTarget,
    type Legend
  } from "$lib/utils";
  import _ from "lodash";
  import { onMount, tick } from "svelte";
  import type { ColumnDefinition, ProgressBarParams } from "tabulator-tables";

  let showAllocation = false;
  let depth = 2;
  let allocationTimelineLegends: Legend[] = [];
  let aggregateLeafNodes: Aggregate[] = [];
  let total = 0;
  let allocationTargets: AllocationTarget[] = [];
  let deposit = 0;

  $: rebalanced = allocationTargets.map((at) => {
    const currentAmount = _.sumBy(_.values(at.aggregates), (a) => a.market_amount);
    const newTotal = total + deposit;
    const targetAmount = newTotal * (at.target / 100);
    const rawDelta = targetAmount - currentAmount;
    const delta = Math.max(rawDelta, -currentAmount);
    const currentPercent = total > 0 ? (currentAmount / total) * 100 : 0;
    const tolerance = total > 0 ? Math.max(total * 0.005, 1) : 0;
    return {
      name: at.name,
      currentAmount,
      currentPercent,
      targetPercent: at.target,
      delta,
      tolerance
    };
  });

  const columns: ColumnDefinition[] = [
    { title: "Account", field: "account", formatter: accountName },
    {
      title: "Market Value",
      field: "market_amount",
      hozAlign: "right",
      formatter: nonZeroCurrency
    },
    {
      title: "Percent",
      field: "percent",
      hozAlign: "right",
      formatter: (cell) => formatPercentage(cell.getValue() / 100, 2)
    },
    {
      title: "%",
      field: "percent",
      hozAlign: "right",
      formatter: "progress",
      cssClass: "has-text-left",
      minWidth: rem(250),
      formatterParams: {
        color: COLORS.assets,
        min: 0
      }
    }
  ];

  onMount(async () => {
    const {
      aggregates: aggregates,
      aggregates_timeline: aggregatesTimeline,
      allocation_targets: targets
    } = await ajax("/api/allocation");
    allocationTargets = targets;
    const accounts = _.keys(aggregates);
    aggregateLeafNodes = _.filter(_.values(aggregates), (a) => a.market_amount > 0);
    total = _.sumBy(aggregateLeafNodes, (a) => a.market_amount);
    aggregateLeafNodes = _.map(aggregateLeafNodes, (a) => {
      a.percent = (a.market_amount / total) * 100;
      return a;
    });
    const max = _.max(_.map(aggregateLeafNodes, (a) => a.percent)) || 100;
    (_.last(columns).formatterParams as ProgressBarParams).max = max;
    const color = generateColorScheme(accounts);
    depth = _.max(_.map(accounts, (account) => account.split(":").length));

    if (!_.isEmpty(allocationTargets)) {
      showAllocation = true;
    }
    await tick();

    renderAllocationTarget(allocationTargets, color);
    renderAllocation(aggregates, color);
    allocationTimelineLegends = renderAllocationTimeline(aggregatesTimeline);
  });
</script>

<section class="section tab-allocation" style={showAllocation ? "" : "display: none"}>
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12 has-text-centered">
        <div class="box overflow-x-auto">
          <div id="d3-allocation-target-treemap" style="width: 100%; position: relative" />
          <svg id="d3-allocation-target" />
        </div>
      </div>
    </div>
    <BoxLabel text="Allocation Targets" />
  </div>
</section>
<section class="section tab-allocation">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12 has-text-centered">
        <div id="d3-allocation-category" style="width: 100%; height: {depth * 100}px" />
      </div>
    </div>
    <BoxLabel text="Allocation by category" />
  </div>
</section>
<section class="section tab-allocation">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12 has-text-centered">
        <div id="d3-allocation-value" style="width: 100%; height: 300px" />
      </div>
    </div>
    <BoxLabel text="Allocation by value" />
  </div>
</section>
<section class="section tab-allocation">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <div class="box">
          <LegendCard legends={allocationTimelineLegends} clazz="ml-4" />
          <svg id="d3-allocation-timeline" width="100%" height="300" />
        </div>
      </div>
    </div>
    <BoxLabel text="Allocation Timeline" />
  </div>
</section>
<section class="section tab-allocation">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <Table data={aggregateLeafNodes} tree {columns} />
      </div>
    </div>
    <BoxLabel text="Allocation Table" />
  </div>
</section>
<section class="section tab-allocation" class:is-hidden={!showAllocation}>
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <div class="box">
          <div class="field mb-4" style="max-width: 320px">
            <label class="label">Amount to invest (negative to withdraw)</label>
            <div class="control">
              <input class="input" type="number" step="1" bind:value={deposit} />
            </div>
          </div>
          <table class="table is-fullwidth is-hoverable">
            <thead>
              <tr>
                <th>Group</th>
                <th class="has-text-right">Current</th>
                <th class="has-text-right">Target</th>
                <th class="has-text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {#each rebalanced as row}
                <tr>
                  <td>{row.name}</td>
                  <td class="has-text-right">
                    {formatCurrency(row.currentAmount)}
                    <span class="has-text-grey is-size-7"
                      >({formatPercentage(row.currentPercent / 100, 1)})</span
                    >
                  </td>
                  <td class="has-text-right">{formatPercentage(row.targetPercent / 100, 1)}</td>
                  <td class="has-text-right">
                    {#if Math.abs(row.delta) > row.tolerance}
                      <span style="color: {row.delta > 0 ? COLORS.gainText : COLORS.lossText}">
                        {row.delta > 0 ? "Buy" : "Sell"}
                        {formatCurrency(Math.abs(row.delta))}
                      </span>
                    {:else}
                      <span class="has-text-grey">—</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    </div>
    <BoxLabel text="Rebalancing Calculator" />
  </div>
</section>
