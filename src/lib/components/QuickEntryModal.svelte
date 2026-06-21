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
  let postingSelections: ({ value: string; label: string } | null)[] = [null, null];

  // SMS parse mode
  let smsMode = false;
  let smsText = "";
  let parsing = false;
  let parseErrorMsg = "";

  $: accountOptions = accounts.map((a) => ({ value: a, label: a }));
  $: fileOptions = files.map((f) => ({ value: f.name, label: f.name }));

  $: if (selectedFileItem) {
    localStorage.setItem(STORAGE_KEY, selectedFileItem.value);
  }

  $: valid =
    payee.trim().length > 0 &&
    selectedFileItem !== null &&
    postings.some((p) => p.amount.trim().length > 0);

  async function loadData() {
    const result = await ajax("/api/editor/files?metadata_only=true");

    files = _.orderBy(result.files, (f) => (f.versions.length > 0 ? f.versions[0] : ""), "desc");
    accounts = result.accounts;
    payees = result.payees;

    applyDefaultFile();
  }

  function applyDefaultFile() {
    const last = localStorage.getItem(STORAGE_KEY);
    if (last && files.find((f) => f.name === last)) {
      selectedFileItem = { value: last, label: last };
      return;
    }

    const journalBasename = USER_CONFIG.journal_path.split("/").pop() ?? "";
    const yyyymm = dayjs().format("YYYYMM");
    const monthFile = files.find((f) => f.name.includes(yyyymm) && f.name !== journalBasename);
    if (monthFile) {
      selectedFileItem = { value: monthFile.name, label: monthFile.name };
      return;
    }

    const first = files.find((f) => f.name !== journalBasename);
    if (first) {
      selectedFileItem = { value: first.name, label: first.name };
    }
  }

  $: if (active) {
    errorMsg = "";
    parseErrorMsg = "";
    loadData();
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

  async function parseSMS() {
    if (!smsText.trim()) return;
    parsing = true;
    parseErrorMsg = "";
    try {
      const tx = await ajax("/api/agent/parse", {
        method: "POST",
        body: JSON.stringify({ text: smsText.trim() })
      });
      if (tx.error) {
        parseErrorMsg = tx.error;
        return;
      }
      // Fill form from parsed result
      if (tx.date) {
        date = tx.date.replace(/-/g, "/");
      }
      if (tx.merchant) {
        payee = tx.merchant;
      }
      const amount = tx.amount ? `${Math.abs(tx.amount)} ${tx.currency || "INR"}` : "";
      const account = tx.suggested_ledger_account || "";
      postings = [
        { account: account, amount: amount },
        { account: "", amount: "" }
      ];
      postingSelections = [
        account ? { value: account, label: account } : null,
        null
      ];
      smsMode = false;
      smsText = "";
    } catch (e: any) {
      parseErrorMsg = e?.message || "Failed to parse";
    } finally {
      parsing = false;
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

      <div class="field">
        <label class="label is-small">Date</label>
        <div class="control">
          <input class="input is-small" type="text" bind:value={date} />
        </div>
      </div>

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

      <label class="label is-small">Postings</label>
      {#each postings as posting, i}
        <div class="field is-grouped mb-2" style="align-items: flex-start">
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
          <div class="control" style="width: 180px; flex-shrink: 0">
            <input
              class="input is-small"
              type="text"
              placeholder="5000 INR"
              bind:value={posting.amount}
            />
          </div>
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
      <button
        class="button is-info"
        disabled={!smsText.trim() || parsing}
        on:click={parseSMS}
      >
        {parsing ? "Parsing…" : "Parse & Fill"}
      </button>
      <button class="button" on:click={() => (active = false)}>Cancel</button>
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
