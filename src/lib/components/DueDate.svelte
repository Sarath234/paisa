<script lang="ts">
  import { dueDateIcon } from "$lib/utils";
  import {
    channelLabel,
    badgeClass,
    badgeIcon,
    type TruthStatus,
    type TruthChannel
  } from "$lib/truthBadge";
  import dayjs from "dayjs";

  export let dueDate: dayjs.Dayjs;
  export let paidDate: dayjs.Dayjs;
  export let dueDateStatus: TruthStatus = "computed";
  export let dueDateChannel: TruthChannel | undefined = undefined;
  export let computedDueDate: dayjs.Dayjs | undefined = undefined;
  export let truthDueDate: dayjs.Dayjs | undefined = undefined;
  export let paidDateStatus: TruthStatus = "computed";
  export let paidDateChannel: TruthChannel | undefined = undefined;
  export let computedPaidDate: dayjs.Dayjs | undefined = undefined;

  $: icon = dueDateIcon(dueDate, paidDate);

  $: activeStatus = paidDate ? paidDateStatus : dueDateStatus;
  $: activeChannel = paidDate ? paidDateChannel : dueDateChannel;
  $: tooltip = paidDate
    ? activeStatus === "corrected"
      ? `Marked paid via ${channelLabel(activeChannel)} (ledger showed ${
          computedPaidDate ? "paid " + computedPaidDate.format("DD MMM") : "still unpaid"
        })`
      : activeStatus === "self_reported"
        ? `Marked paid via Telegram (unconfirmed by bank)`
        : `Confirmed via ${channelLabel(activeChannel)}`
    : activeStatus === "corrected"
      ? `Due date corrected to ${truthDueDate?.format("DD MMM")} via ${channelLabel(
          activeChannel
        )} (computed: ${computedDueDate?.format("DD MMM")})`
      : `Confirmed via ${channelLabel(activeChannel)}`;
</script>

<span title="due on {dueDate.format('DD MMM YYYY')}">
  <span class="icon is-small {icon.color}">
    <i class="fas {icon.icon}" />
  </span>
  {#if paidDate}
    <span>paid on {paidDate.format("DD MMM YYYY")}</span>
  {:else}
    <span>due {dueDate.fromNow()}</span>
  {/if}
  {#if activeStatus !== "computed"}
    <span class="tag is-rounded is-small ml-1 {badgeClass(activeStatus)}" title={tooltip}>
      <i class="fas {badgeIcon(activeStatus)} is-size-7" />
    </span>
  {/if}
</span>
