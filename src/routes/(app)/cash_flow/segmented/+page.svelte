<script lang="ts">
  import { onMount } from "svelte";
  import _ from "lodash";
  import { renderSegmentedFlow, segmentedNodeDepth, segmentedNodeRoot } from "$lib/cash_flow";
  import { ajax, rem, type Graph, type Legend, type Posting } from "$lib/utils";
  import { dateMin, year } from "../../../../store";
  import {
    setCashflowDepthAllowed,
    cashflowExpenseDepth,
    cashflowIncomeDepth
  } from "../../../../persisted_store";
  import ZeroState from "$lib/components/ZeroState.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";

  let legends: Legend[] = [];
  let segmented_graph: Record<string, Graph>, expenses: Posting[];
  let isEmpty = false;

  function maxDepth(rootType: string) {
    if (!segmented_graph) return 1;
    const max = _.chain(segmented_graph)
      .flatMap((g) => g.nodes)
      .filter((n) => segmentedNodeRoot(n.name) === rootType)
      .map((n) => segmentedNodeDepth(n.name))
      .max()
      .value();
    return max || 1;
  }

  function filter(graph: Graph, incomeDepth: number, expenseDepth: number) {
    if (!graph) return graph;
    const [removed, allowed] = _.partition(graph.nodes, (n) => {
      const root = segmentedNodeRoot(n.name);
      const d = segmentedNodeDepth(n.name);
      if (root === "Income") return d > incomeDepth;
      if (root === "Expenses") return d > expenseDepth;
      return false;
    });
    const removedIds = removed.map((n) => n.id);
    return {
      nodes: allowed,
      links: graph.links.filter(
        (l) => !removedIds.includes(l.source) && !removedIds.includes(l.target)
      )
    };
  }

  $: if (segmented_graph) {
    if (segmented_graph[$year] == null) {
      isEmpty = true;
    } else {
      legends = renderSegmentedFlow(
        filter(_.cloneDeep(segmented_graph[$year]), $cashflowIncomeDepth, $cashflowExpenseDepth),
        "d3-segmented-flow"
      );
      isEmpty = false;
    }
  }

  onMount(async () => {
    ({ expenses, segmented_graph } = await ajax("/api/expense"));
    const firstExpense = _.minBy(expenses, (e) => e.date);
    if (firstExpense) {
      dateMin.set(firstExpense.date);
    }
    setCashflowDepthAllowed(maxDepth("Expenses"), maxDepth("Income"));
  });
</script>

<section class="section" style="padding-bottom: 0 !important">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <div class="box overflow-x-auto">
          <ZeroState item={!isEmpty}
            ><strong>Oops!</strong> You have not made any transactions for the selected year.</ZeroState
          >

          <LegendCard {legends} clazz="ml-5 mb-2" />
          <svg
            class:is-not-visible={isEmpty}
            id="d3-segmented-flow"
            height={window.innerHeight - rem(210)}
          />
        </div>
      </div>
    </div>
  </div>
</section>
