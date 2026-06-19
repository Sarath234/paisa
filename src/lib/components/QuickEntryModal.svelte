<script lang="ts">
  import Select from "svelte-select";
  import Modal from "$lib/components/Modal.svelte";
  import { ajax, type LedgerFile } from "$lib/utils";
  import { appendTransaction } from "$lib/editor";
  import { format } from "$lib/journal";
  import * as toast from "bulma-toast";
  import _ from "lodash";
  import dayjs from "dayjs";

  export let active = false;

  const STORAGE_KEY = "paisa:quickentry:lastFile";

  interface PostingRow {
    account: string;
    amount: string;
  }

  interface Entry {
    date: string;
    desc: string;
    src: string;
    amt: string;
    dest: string;
  }

  let files: LedgerFile[] = [];
  let accounts: string[] = [];
  let payees: string[] = [];
  let saving = false;
  let errorMsg = "";

  let selectedFileItem: { value: string; label: string } | null = null;
  let date = dayjs().format("YYYY/MM/DD");
  let payee = "";
  let postings: PostingRow[] = [
    { account: "", amount: "" },
    { account: "", amount: "" }
  ];
  // Parallel array of svelte-select selections for account fields
  let postingSelections: ({ value: string; label: string } | null)[] = [null, null];

  // SMS parse mode
  let smsMode = false;
  let smsText = "";
  let parsing = false;
  let parseErrorMsg = "";

  // Approval card state machine: idle | approval | editing
  type ApprovalState = "idle" | "approval" | "editing";
  let approvalState: ApprovalState = "idle";
  let parsedEntry: Entry | null = null;
  let chosenDest = "";
  let editingDestItem: { value: string; label: string } | null = null;
  let isDuplicate = false;
  let postError = "";
  let posting = false;

  $: accountOptions = accounts.map((a) => ({ value: a, label: a }));
  $: fileOptions = files.map((f) => ({ value: f.name, label: f.name }));

  // Persist last-selected file to localStorage
  $: if (selectedFileItem) {
    localStorage.setItem(STORAGE_KEY, selectedFileItem.value);
  }

  $: valid =
    payee.trim().length > 0 &&
    selectedFileItem !== null &&
    postings.some((p) => p.amount.trim().length > 0);

  $: if (active) {
    errorMsg = "";
    parseErrorMsg = "";
    resetApproval();
    loadData();
  }

  async function loadData() {
    const result = await ajax("/api/editor/files?metadata_only=true");

    // Sort by most recent backup descending; files with no backups go last
    files = _.orderBy(result.files, (f) => (f.versions.length > 0 ? f.versions[0] : ""), "desc");
    accounts = result.accounts;
    payees = result.payees;

    applyDefaultFile();
  }

  function applyDefaultFile() {
    // 1. Restore last-selected file from localStorage
    const last = localStorage.getItem(STORAGE_KEY);
    if (last && files.find((f) => f.name === last)) {
      selectedFileItem = { value: last, label: last };
      return;
    }

    // 2. Current month file (YYYYMM in path), excluding root journal file
    const journalBasename = USER_CONFIG.journal_path.split("/").pop() ?? "";
    const yyyymm = dayjs().format("YYYYMM");
    const monthFile = files.find((f) => f.name.includes(yyyymm) && f.name !== journalBasename);
    if (monthFile) {
      selectedFileItem = { value: monthFile.name, label: monthFile.name };
      return;
    }

    // 3. Most recently edited non-root file
    const first = files.find((f) => f.name !== journalBasename);
    if (first) {
      selectedFileItem = { value: first.name, label: first.name };
    }
  }

  function addPosting() {
    postings = [...postings, { account: "", amount: "" }];
    postingSelections = [...postingSelections, null];
  }

  function removePosting(i: number) {
    postings = postings.filter((_, idx) => idx !== i);
    postingSelections = postingSelections.filter((_, idx) => idx !== i);
  }

  function updateAccount(i: number, value: string) {
    postings[i] = { ...postings[i], account: value };
    postingSelections[i] = value ? { value, label: value } : null;
    postings = postings;
    postingSelections = postingSelections;
  }

  function buildTransactionText(): string {
    const lines = [`${date} ${payee.trim()}`];
    for (const p of postings) {
      if (p.account.trim()) {
        const line = p.amount.trim()
          ? `    ${p.account.trim()}    ${p.amount.trim()}`
          : `    ${p.account.trim()}`;
        lines.push(line);
      }
    }
    return format(lines.join("\n"));
  }

  function resetForm() {
    payee = "";
    postings = [
      { account: "", amount: "" },
      { account: "", amount: "" }
    ];
    postingSelections = [null, null];
  }

  function resetApproval() {
    approvalState = "idle";
    parsedEntry = null;
    chosenDest = "";
    editingDestItem = null;
    isDuplicate = false;
    postError = "";
    smsText = "";
  }

  async function parseSMS() {
    if (!smsText.trim()) return;
    parsing = true;
    parseErrorMsg = "";
    try {
      const result = await ajax("/api/agent/parse", {
        method: "POST",
        body: JSON.stringify({ text: smsText.trim() })
      });
      if (result.error) {
        parseErrorMsg = result.error;
        return;
      }
      parsedEntry = result as Entry;
      chosenDest = parsedEntry?.dest ?? "";
      editingDestItem = chosenDest ? { value: chosenDest, label: chosenDest } : null;
      approvalState = "approval";
    } catch (e: any) {
      parseErrorMsg = e?.message || "Failed to parse";
    } finally {
      parsing = false;
    }
  }

  async function postTransaction(force = false) {
    if (!parsedEntry) return;
    posting = true;
    postError = "";
    try {
      const result = await ajax("/api/agent/post", {
        method: "POST",
        body: JSON.stringify({ entry: parsedEntry, dest: chosenDest, force })
      });
      if (result.error) {
        postError = result.error;
        return;
      }
      if (result.status === "duplicate") {
        isDuplicate = true;
        return;
      }
      toast.toast({ message: "Transaction posted", type: "is-success" });
      resetApproval();
    } catch (e: any) {
      postError = e?.message || "Failed to post";
    } finally {
      posting = false;
    }
  }

  async function save(addAnother = false) {
    if (!valid || !selectedFileItem) return;
    saving = true;
    errorMsg = "";
    try {
      await appendTransaction(selectedFileItem.value, buildTransactionText());
      toast.toast({ message: `Added to ${selectedFileItem.value}`, type: "is-success" });
      if (addAnother) {
        resetForm();
      } else {
        active = false;
      }
    } catch (e: any) {
      errorMsg = e?.message || "Failed to save";
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:active width="min(680px, 100vw)">
  <svelte:fragment slot="head">
    <p class="modal-card-title">Add Transaction</p>
    <div class="tabs is-small mb-0 ml-4" style="align-self: center">
      <ul>
        <li class:is-active={!smsMode}>
          <a role="button" tabindex="0" on:click={() => { smsMode = false; parseErrorMsg = ""; }}>Manual</a>
        </li>
        <li class:is-active={smsMode}>
          <a role="button" tabindex="0" on:click={() => { smsMode = true; errorMsg = ""; }}>
            <span class="icon is-small"><i class="fas fa-robot" /></span>
            <span>Parse SMS</span>
          </a>
        </li>
      </ul>
    </div>
  </svelte:fragment>

  <svelte:fragment slot="body">
    {#if smsMode}
      <!-- SMS parse panel -->
      {#if approvalState === "idle"}
        {#if parseErrorMsg}
          <div class="notification is-danger is-light mb-3 py-2">{parseErrorMsg}</div>
        {/if}
        <div class="field">
          <label class="label is-small">Bank SMS / notification text</label>
          <div class="control">
            <textarea
              class="textarea is-small"
              rows="5"
              placeholder="Paste the bank SMS or email notification here…"
              bind:value={smsText}
            />
          </div>
        </div>
      {:else if approvalState === "approval" || approvalState === "editing"}
        <!-- Approval card -->
        {#if isDuplicate}
          <div class="notification is-warning is-light mb-3 py-2">
            Already imported — this looks like a duplicate.
          </div>
        {/if}
        {#if postError}
          <div class="notification is-danger is-light mb-3 py-2">{postError}</div>
        {/if}
        <div class="box" style="border: 1px solid {approvalState === 'editing' ? '#3273dc' : '#dbdbdb'}; padding: 1rem;">
          <p class="has-text-weight-semibold">{parsedEntry?.desc ?? "—"}</p>
          <p class="is-size-7 has-text-grey">{parsedEntry?.date ?? ""}</p>
          <p class="is-size-4 has-text-danger has-text-weight-semibold mt-2">
            {parsedEntry?.amt ?? "—"}
          </p>
          <div class="mt-3 pt-3" style="border-top: 1px solid #ebebeb;">
            <p class="is-size-7 has-text-grey-light" style="text-transform:uppercase;letter-spacing:.05em;">Source Account</p>
            <p class="is-family-monospace is-size-7 mt-1">{parsedEntry?.src ?? "—"}</p>
          </div>
          <div class="mt-3 pt-3" style="border-top: 1px solid #ebebeb;">
            <p class="is-size-7 has-text-grey-light" style="text-transform:uppercase;letter-spacing:.05em;">Destination Account</p>
            {#if approvalState === "editing"}
              <Select
                items={accountOptions}
                bind:value={editingDestItem}
                showChevron={true}
                searchable={true}
                clearable={false}
                placeholder="Search accounts…"
                floatingConfig={{ strategy: "fixed" }}
                on:change={(e) => {
                  chosenDest = e.detail?.value ?? "";
                  editingDestItem = e.detail ?? null;
                }}
              />
            {:else}
              <p class="is-family-monospace is-size-7 mt-1">{chosenDest || "—"}</p>
            {/if}
          </div>
        </div>
        <p class="is-size-7 has-text-grey mt-1" style="text-align:right;">
          <a role="button" tabindex="0" on:click={resetApproval} on:keydown={(e) => e.key === 'Enter' && resetApproval()}>← paste another message</a>
        </p>
      {/if}
    {:else}
      <!-- Manual entry panel -->
      {#if errorMsg}
        <div class="notification is-danger is-light mb-3 py-2">{errorMsg}</div>
      {/if}

      <div class="field">
        <label class="label is-small">File</label>
        <div class="control">
          <Select
            items={fileOptions}
            bind:value={selectedFileItem}
            showChevron={true}
            searchable={true}
            clearable={false}
            floatingConfig={{ strategy: "fixed" }}
          />
        </div>
      </div>

      <!-- Date -->
      <div class="field">
        <label class="label is-small">Date</label>
        <div class="control">
          <input class="input is-small" type="text" bind:value={date} />
        </div>
      </div>

      <!-- Payee: plain text input + datalist (free-form entry is primary use case) -->
      <div class="field">
        <label class="label is-small">Payee</label>
        <div class="control">
          <input
            class="input is-small"
            type="text"
            placeholder="e.g. Swiggy, Amazon"
            bind:value={payee}
            list="quick-entry-payees"
          />
          <datalist id="quick-entry-payees">
            {#each payees as p}
              <option value={p} />
            {/each}
          </datalist>
        </div>
      </div>

      <!-- Posting rows -->
      <label class="label is-small">Postings</label>
      {#each postings as posting, i}
        <div class="field is-grouped mb-2" style="align-items: flex-start">
          <!-- Account: svelte-select searchable dropdown -->
          <div class="control is-expanded">
            <Select
              items={accountOptions}
              value={postingSelections[i]}
              showChevron={false}
              searchable={true}
              clearable={true}
              placeholder="Account"
              floatingConfig={{ strategy: "fixed" }}
              on:change={(e) => updateAccount(i, e.detail?.value ?? "")}
              on:clear={() => updateAccount(i, "")}
            />
          </div>
          <!-- Amount -->
          <div class="control" style="width: 180px; flex-shrink: 0">
            <input
              class="input is-small"
              type="text"
              placeholder="5000 INR"
              bind:value={posting.amount}
            />
          </div>
          <!-- Remove button (only when > 2 rows) -->
          {#if postings.length > 2}
            <div class="control">
              <button
                class="button is-small is-light is-danger"
                on:click={() => removePosting(i)}
                title="Remove posting"
              >
                <span class="icon is-small"><i class="fas fa-times" /></span>
              </button>
            </div>
          {/if}
        </div>
      {/each}

      <a class="is-size-7 has-text-link" role="button" tabindex="0" on:click={addPosting}>
        + Add posting
      </a>
    {/if}
  </svelte:fragment>

  <svelte:fragment slot="foot">
    {#if smsMode}
      {#if approvalState === "idle"}
        <button class="button is-info" disabled={!smsText.trim() || parsing} on:click={parseSMS}>
          {parsing ? "Parsing…" : "Parse & Review"}
        </button>
      {:else if approvalState === "approval"}
        <button
          class="button is-success"
          disabled={posting}
          on:click={() => postTransaction(isDuplicate)}
        >
          {posting ? "Posting…" : isDuplicate ? "Post Anyway" : "✓ Post"}
        </button>
        <button
          class="button is-info is-light"
          on:click={() => { isDuplicate = false; postError = ""; approvalState = "editing"; }}
        >
          ✎ Edit
        </button>
        <button class="button" on:click={resetApproval}>✗ Skip</button>
      {:else if approvalState === "editing"}
        <button class="button is-success" disabled={posting} on:click={() => postTransaction(false)}>
          {posting ? "Posting…" : "✓ Post"}
        </button>
        <button class="button" on:click={() => { chosenDest = parsedEntry?.dest ?? ""; editingDestItem = chosenDest ? { value: chosenDest, label: chosenDest } : null; approvalState = "approval"; }}>
          Cancel
        </button>
      {/if}
    {:else}
      <button class="button is-success" disabled={!valid || saving} on:click={() => save(false)}>
        {saving ? "Saving…" : "Save"}
      </button>
      <button class="button" disabled={!valid || saving} on:click={() => save(true)}>
        Save &amp; Add Another
      </button>
      <button class="button" on:click={() => (active = false)}>Cancel</button>
    {/if}
  </svelte:fragment>
</Modal>
