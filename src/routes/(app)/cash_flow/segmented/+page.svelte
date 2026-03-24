<script lang="ts">
  import { onMount } from "svelte";
  import _ from "lodash";
  import { renderSegmentedFlow } from "$lib/cash_flow";
  import { ajax, rem, type Graph, type Legend, type Posting } from "$lib/utils";
  import { dateMin, year } from "../../../../store";
  import ZeroState from "$lib/components/ZeroState.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";

  let legends: Legend[] = [];
  let segmented_graph: Record<string, Graph>, expenses: Posting[];
  let isEmpty = false;

  $: if (segmented_graph) {
    if (segmented_graph[$year] == null) {
      isEmpty = true;
    } else {
      legends = renderSegmentedFlow(_.cloneDeep(segmented_graph[$year]), "d3-segmented-flow");
      isEmpty = false;
    }
  }

  onMount(async () => {
    ({ expenses, segmented_graph } = await ajax("/api/expense"));
    let firstExpense = _.minBy(expenses, (e) => e.date);
    if (firstExpense) {
      dateMin.set(firstExpense.date);
    }
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
