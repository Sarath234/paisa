<!-- src/lib/components/StatementUploadModal.svelte -->
<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import Dropzone from "svelte-file-dropzone";
  import { uploadRows, newRowId, type UploadRow } from "$lib/stores/statementUpload";

  export let active = false;

  const POLL_MS = 3000;
  const TIMEOUT_MS = 5 * 60 * 1000;
  let banner = "";
  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let hidden = false;

  function handleVisibility() {
    hidden = document.hidden;
  }
  // document only exists in the browser — never touch it during SSR init.
  onMount(() => {
    document.addEventListener("visibilitychange", handleVisibility);
    return () => document.removeEventListener("visibilitychange", handleVisibility);
  });
  onDestroy(() => {
    stopPolling();
  });

  function startPolling() {
    if (pollTimer) return;
    pollTimer = setInterval(pollPending, POLL_MS);
  }
  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  async function pollPending() {
    if (hidden) return;
    const rows = $uploadRows.filter((r) => r.status === "queued");
    if (rows.length === 0) {
      stopPolling();
      return;
    }
    for (const row of rows) {
      if (Date.now() - row.startedAt > TIMEOUT_MS) {
        update(row.id, { status: "timeout" });
        continue;
      }
      try {
        const res = await fetch(`/api/agent/statement/status?file=${encodeURIComponent(row.name)}`);
        const body = await res.json();
        if (!res.ok) continue; // transient; keep polling
        if (body.status === "done") update(row.id, { status: "done", summary: body.summary });
        else if (body.status === "failed") update(row.id, { status: "failed" });
        else if (body.status === "unknown") update(row.id, { status: "unknown" });
      } catch {
        // agent/paisa briefly unreachable — keep polling until timeout
      }
    }
  }

  function update(id: number, patch: Partial<UploadRow>) {
    uploadRows.update((rows) => rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }

  async function handleDrop(e: CustomEvent<{ acceptedFiles: File[] }>) {
    banner = "";
    for (const file of e.detail.acceptedFiles) {
      const id = newRowId();
      uploadRows.update((rows) => [
        {
          id,
          name: file.name,
          size: file.size,
          status: "uploading" as const,
          startedAt: Date.now()
        },
        ...rows
      ]);
      const fd = new FormData();
      fd.append("file", file);
      try {
        const res = await fetch("/api/agent/statement/upload", { method: "POST", body: fd });
        // Parse defensively: an error response (e.g. a 502/503 from a proxy in front of
        // the agent) may not have a JSON body at all. Keep res.status available even
        // when parsing fails, so the status-based banner logic below still fires.
        let body: any = {};
        try {
          body = await res.json();
        } catch {
          // non-JSON body — fall through with body = {}
        }
        if (!res.ok) {
          const msg = body.error || `upload failed (${res.status})`;
          update(id, { status: "error", error: msg });
          if (res.status === 502 || res.status === 503) banner = msg;
          continue;
        }
        update(id, { name: body.file, status: "queued", startedAt: Date.now() });
        startPolling();
      } catch (err) {
        update(id, { status: "error", error: "paisa server unreachable" });
      }
    }
  }

  function chip(row: UploadRow): string {
    switch (row.status) {
      case "uploading":
        return "⏫ uploading…";
      case "queued":
        return "⏳ queued";
      case "done":
        return row.summary
          ? `✓ ${row.summary.matched} matched, ${row.summary.missing} missing, ${row.summary.extra} extra`
          : "✓ processed (details on Telegram / doctor page)";
      case "failed":
        return "✗ failed — see Telegram for the reason";
      case "unknown":
        return "? not found — was the file removed?";
      case "timeout":
        return "⏳ still queued after 5m — check the doctor page";
      case "error":
        return `✗ ${row.error}`;
    }
  }
  $: if (active) startPolling();
</script>

<div class="modal" class:is-active={active}>
  <div class="modal-background" on:click={() => (active = false)} />
  <div class="modal-card">
    <header class="modal-card-head">
      <p class="modal-card-title">Drop statement PDFs</p>
      <button class="delete" aria-label="close" on:click={() => (active = false)} />
    </header>
    <section class="modal-card-body">
      {#if banner}
        <div class="notification is-warning is-light">{banner}</div>
      {/if}
      <Dropzone accept=".pdf" on:drop={handleDrop}>
        <p>
          Drag statement PDFs here, or click to choose. They are matched to accounts by filename,
          processed by the agent, and reconciled — results appear below and on Telegram.
        </p>
      </Dropzone>
      {#if $uploadRows.length > 0}
        <table class="table is-fullwidth is-narrow mt-4">
          <tbody>
            {#each $uploadRows as row (row.id)}
              <tr>
                <td class="is-family-monospace">{row.name}</td>
                <td>{(row.size / 1024).toFixed(0)} KB</td>
                <td>{chip(row)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
  </div>
</div>
