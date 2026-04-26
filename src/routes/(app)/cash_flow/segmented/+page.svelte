<script lang="ts">
  import { onMount } from "svelte";
  import _ from "lodash";
  import dayjs from "dayjs";
  import { renderSegmentedFlow, segmentedNodeDepth, segmentedNodeRoot } from "$lib/cash_flow";
  import { ajax, rem, type Graph, type Legend, type Posting } from "$lib/utils";
  import { dateMin } from "../../../../store";
  import {
    setCashflowDepthAllowed,
    cashflowExpenseDepth,
    cashflowIncomeDepth
  } from "../../../../persisted_store";
  import ZeroState from "$lib/components/ZeroState.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";

  type ViewMode = "week" | "month" | "year";

  let legends: Legend[] = [];
  let segmented_graph: Record<string, Graph> = {};
  let segmented_graph_monthly: Record<string, Graph> = {};
  let segmented_graph_weekly: Record<string, Graph> = {};
  let expenses: Posting[];
  let isEmpty = false;

  let viewMode: ViewMode = "year";
  let currentPeriod = "";

  function sortedPeriods(mode: ViewMode): string[] {
    const g =
      mode === "year"
        ? segmented_graph
        : mode === "month"
          ? segmented_graph_monthly
          : segmented_graph_weekly;
    return Object.keys(g).sort();
  }

  function currentGraph(mode: ViewMode, period: string): Graph | undefined {
    if (mode === "year") return segmented_graph[period];
    if (mode === "month") return segmented_graph_monthly[period];
    return segmented_graph_weekly[period];
  }

  function periodLabel(mode: ViewMode, period: string): string {
    if (mode === "year") return period;
    if (mode === "month") return dayjs(period, "YYYY-MM").format("MMMM YYYY");
    // "2024-W03" → find Monday of that ISO week
    const [yearStr, weekStr] = period.split("-W");
    const isoYear = parseInt(yearStr);
    const isoWeek = parseInt(weekStr);
    const jan4 = dayjs(`${isoYear}-01-04`);
    const daysToMonday = jan4.day() === 0 ? 6 : jan4.day() - 1;
    const weekStart = jan4.subtract(daysToMonday, "day").add((isoWeek - 1) * 7, "day");
    const weekEnd = weekStart.add(6, "day");
    if (weekStart.year() === weekEnd.year()) {
      return `${weekStart.format("MMM D")} – ${weekEnd.format("MMM D, YYYY")}`;
    }
    return `${weekStart.format("MMM D, YYYY")} – ${weekEnd.format("MMM D, YYYY")}`;
  }

  function navigate(delta: number) {
    const periods = sortedPeriods(viewMode);
    const idx = periods.indexOf(currentPeriod);
    const next = idx + delta;
    if (next >= 0 && next < periods.length) {
      currentPeriod = periods[next];
    }
  }

  function setView(mode: ViewMode) {
    viewMode = mode;
    const periods = sortedPeriods(mode);
    currentPeriod = periods[periods.length - 1] ?? "";
  }

  function maxDepth(rootType: string, graph: Record<string, Graph>) {
    if (!graph) return 1;
    const max = _.chain(graph)
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

  $: periods = sortedPeriods(viewMode);
  $: currentIdx = periods.indexOf(currentPeriod);
  $: canPrev = currentIdx > 0;
  $: canNext = currentIdx < periods.length - 1;

  $: if (currentPeriod) {
    const graph = currentGraph(viewMode, currentPeriod);
    if (!graph) {
      isEmpty = true;
    } else {
      legends = renderSegmentedFlow(
        filter(_.cloneDeep(graph), $cashflowIncomeDepth, $cashflowExpenseDepth),
        "d3-segmented-flow"
      );
      isEmpty = false;
    }
  }

  onMount(async () => {
    ({ expenses, segmented_graph, segmented_graph_monthly, segmented_graph_weekly } =
      await ajax("/api/expense"));
    const firstExpense = _.minBy(expenses, (e) => e.date);
    if (firstExpense) {
      dateMin.set(firstExpense.date);
    }
    setCashflowDepthAllowed(
      maxDepth("Expenses", segmented_graph),
      maxDepth("Income", segmented_graph)
    );
    setView("year");
  });
</script>

<section class="section" style="padding-bottom: 0 !important">
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <div class="box overflow-x-auto">
          <div class="is-flex is-align-items-center is-justify-content-space-between mb-4">
            <div class="tabs is-toggle is-small mb-0">
              <ul>
                <li class:is-active={viewMode === "week"}>
                  <a on:click|preventDefault={() => setView("week")} href="#">Week</a>
                </li>
                <li class:is-active={viewMode === "month"}>
                  <a on:click|preventDefault={() => setView("month")} href="#">Month</a>
                </li>
                <li class:is-active={viewMode === "year"}>
                  <a on:click|preventDefault={() => setView("year")} href="#">Year</a>
                </li>
              </ul>
            </div>

            <div class="is-flex is-align-items-center" style="gap: 0.75rem">
              <button
                class="button is-small"
                disabled={!canPrev}
                on:click={() => navigate(-1)}
                aria-label="Previous period"
              >
                ‹
              </button>
              <span class="has-text-weight-semibold" style="min-width: 14rem; text-align: center">
                {currentPeriod ? periodLabel(viewMode, currentPeriod) : ""}
              </span>
              <button
                class="button is-small"
                disabled={!canNext}
                on:click={() => navigate(1)}
                aria-label="Next period"
              >
                ›
              </button>
            </div>
          </div>

          <ZeroState item={!isEmpty}>
            <strong>Oops!</strong> No transactions found for this period.
          </ZeroState>

          <LegendCard {legends} clazz="ml-5 mb-2" />
          <svg
            class:is-not-visible={isEmpty}
            id="d3-segmented-flow"
            height={window.innerHeight - rem(260)}
          />
        </div>
      </div>
    </div>
  </div>
</section>
