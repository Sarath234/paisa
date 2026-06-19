<script lang="ts">
  import { afterUpdate, tick } from "svelte";
  import { chatMessages, appendMessage, clearHistory } from "$lib/stores/chat";

  let input = "";
  let loading = false;
  let threadEl: HTMLDivElement;

  afterUpdate(() => {
    threadEl?.scrollTo({ top: threadEl.scrollHeight });
  });

  async function send() {
    const text = input.trim();
    if (!text || loading) return;
    input = "";

    appendMessage({ id: crypto.randomUUID(), role: "user", text, ts: Date.now() });
    loading = true;
    await tick();

    try {
      const res = await fetch("/api/agent/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: text })
      });
      const data = await res.json();
      let replyText: string;
      if (res.ok && data.reply) {
        replyText = data.reply;
      } else if (res.status === 502) {
        replyText = "⚠️ paisa-agent is not running. Start it to use the assistant.";
      } else if (res.status === 422) {
        replyText = '⚠️ Couldn\'t understand that — try rephrasing, e.g. "food spend this month"';
      } else {
        replyText = data.error ?? "⚠️ Something went wrong.";
      }
      appendMessage({
        id: crypto.randomUUID(),
        role: "assistant",
        text: replyText,
        ts: Date.now()
      });
    } catch {
      appendMessage({
        id: crypto.randomUUID(),
        role: "assistant",
        text: "⚠️ Network error — check your connection.",
        ts: Date.now()
      });
    } finally {
      loading = false;
    }
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }
</script>

<section
  class="section"
  style="height: calc(100vh - 52px); display: flex; flex-direction: column; padding-bottom: 0;"
>
  <!-- Header -->
  <div class="level mb-3" style="flex-shrink: 0;">
    <div class="level-left">
      <h4 class="title is-4 mb-0">
        <span class="icon mr-2"><i class="fas fa-comment-dots" /></span>Finance Assistant
      </h4>
    </div>
    <div class="level-right">
      <button class="button is-small is-light" on:click={clearHistory}>
        <span class="icon is-small"><i class="fas fa-trash-can" /></span>
        <span>Clear history</span>
      </button>
    </div>
  </div>

  <!-- Thread -->
  <div
    bind:this={threadEl}
    style="flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 0.6rem; padding-bottom: 1rem;"
  >
    {#if $chatMessages.length === 0}
      <div style="text-align: center; color: #aaa; padding: 3rem 1rem;">
        <p class="is-size-5 mb-3">💬</p>
        <p>Ask anything about your finances.</p>
        <p class="is-size-7 mt-2" style="color: #bbb;">
          Try: "how much did I spend on food this month?" · "what's my net worth?" · "am I over
          budget?"
        </p>
      </div>
    {/if}
    {#each $chatMessages as msg (msg.id)}
      <div
        style="display: flex; justify-content: {msg.role === 'user' ? 'flex-end' : 'flex-start'};"
      >
        <div
          style="
            max-width: 70%;
            padding: 0.5rem 0.85rem;
            border-radius: 1rem;
            font-size: 0.9rem;
            line-height: 1.5;
            white-space: pre-wrap;
            word-break: break-word;
            {msg.role === 'user'
            ? 'background: #3273dc; color: white; border-bottom-right-radius: 0.2rem;'
            : 'background: #f5f5f5; color: #363636; border-bottom-left-radius: 0.2rem;'}
          "
        >
          {msg.text}
        </div>
      </div>
    {/each}
    {#if loading}
      <div style="display: flex; justify-content: flex-start;">
        <div
          style="background: #f5f5f5; border-radius: 1rem; padding: 0.5rem 0.85rem; color: #aaa; font-size: 0.85rem;"
        >
          ···
        </div>
      </div>
    {/if}
  </div>

  <!-- Input bar -->
  <div
    class="field has-addons mb-0"
    style="flex-shrink: 0; padding: 0.75rem 0; border-top: 1px solid #ededed;"
  >
    <div class="control is-expanded">
      <input
        class="input"
        bind:value={input}
        on:keydown={handleKey}
        placeholder="Ask about your finances…"
        disabled={loading}
        autofocus
      />
    </div>
    <div class="control">
      <button class="button is-primary" on:click={send} disabled={loading || !input.trim()}>
        <span class="icon"><i class="fas fa-paper-plane" /></span>
      </button>
    </div>
  </div>
</section>
