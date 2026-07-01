<script lang="ts">
  import { afterUpdate, tick } from "svelte";
  import { chatMessages, appendMessage, generateId, type ChatMessage } from "$lib/stores/chat";
  import { theme } from "../../store";

  $: isDark = $theme === "dark";

  let open = false;
  let input = "";
  let loading = false;
  let threadEl: HTMLDivElement;

  $: visibleMessages = $chatMessages.slice(-8);

  afterUpdate(() => {
    if (open && threadEl) {
      threadEl.scrollTop = threadEl.scrollHeight;
    }
  });

  async function send() {
    const text = input.trim();
    if (!text || loading) return;
    input = "";

    appendMessage({ id: generateId(), role: "user", text, ts: Date.now() });
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
      appendMessage({ id: generateId(), role: "assistant", text: replyText, ts: Date.now() });
    } catch {
      appendMessage({
        id: generateId(),
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

<!-- FAB -->
<button class="chat-fab" aria-label="Finance assistant" on:click={() => (open = !open)}>
  <span class="icon"><i class="fas {open ? 'fa-times' : 'fa-comment-dots'}" /></span>
</button>

<!-- Popup bubble -->
{#if open}
  <div class="chat-popup" class:dark={isDark}>
    <!-- Header -->
    <div class="chat-popup-header">
      <span class="has-text-weight-semibold">💬 Assistant</span>
      <div class="chat-popup-header-actions">
        <a
          href="/assistant"
          class="has-text-white"
          title="Open full page"
          on:click={() => (open = false)}
        >
          <span class="icon is-small"><i class="fas fa-up-right-from-square" /></span>
        </a>
        <button class="delete is-small" on:click={() => (open = false)} aria-label="Close" />
      </div>
    </div>

    <!-- Thread -->
    <div class="chat-thread" bind:this={threadEl}>
      {#if visibleMessages.length === 0}
        <p class="chat-hint">
          Ask anything about your finances — spending, net worth, account balances, budget status.
        </p>
      {/if}
      {#each visibleMessages as msg (msg.id)}
        <div
          class="chat-bubble {msg.role === 'user' ? 'chat-bubble-user' : 'chat-bubble-assistant'}"
        >
          {msg.text}
        </div>
      {/each}
      {#if loading}
        <div class="chat-bubble chat-bubble-assistant chat-typing">
          <span>●</span><span>●</span><span>●</span>
        </div>
      {/if}
    </div>

    <!-- Input -->
    <div class="chat-input-row">
      <input
        class="input is-small"
        bind:value={input}
        on:keydown={handleKey}
        placeholder="Ask anything…"
        disabled={loading}
      />
      <button
        class="button is-primary is-small"
        on:click={send}
        disabled={loading || !input.trim()}
      >
        <span class="icon is-small"><i class="fas fa-paper-plane" /></span>
      </button>
    </div>
  </div>
{/if}

<style>
  .chat-fab {
    position: fixed;
    bottom: 1.5rem;
    right: 1.5rem;
    z-index: 40;
    width: 3rem;
    height: 3rem;
    border-radius: 50%;
    background: #3273dc;
    color: white;
    border: none;
    cursor: pointer;
    box-shadow: 0 3px 10px rgba(50, 115, 220, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.1rem;
    transition: background 0.15s;
  }
  .chat-fab:hover {
    background: #2160c4;
  }

  .chat-popup {
    position: fixed;
    bottom: 5.5rem;
    right: 1.5rem;
    z-index: 40;
    width: 360px;
    max-height: 480px;
    background: white;
    border: 1px solid #dbdbdb;
    border-radius: 12px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.14);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .chat-popup-header {
    background: #3273dc;
    color: white;
    padding: 0.6rem 0.9rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.9rem;
    flex-shrink: 0;
  }
  .chat-popup-header-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .chat-popup-header-actions .delete {
    background: rgba(255, 255, 255, 0.3);
  }
  .chat-popup-header-actions .delete:hover {
    background: rgba(255, 255, 255, 0.5);
  }

  .chat-thread {
    flex: 1;
    overflow-y: auto;
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-height: 0;
  }

  .chat-hint {
    font-size: 0.8rem;
    color: #aaa;
    text-align: center;
    padding: 1rem 0.5rem;
  }

  .chat-bubble {
    max-width: 80%;
    padding: 0.45rem 0.75rem;
    border-radius: 1rem;
    font-size: 0.85rem;
    line-height: 1.45;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .chat-bubble-user {
    align-self: flex-end;
    background: #3273dc;
    color: white;
    border-bottom-right-radius: 0.2rem;
  }
  .chat-bubble-assistant {
    align-self: flex-start;
    background: #f5f5f5;
    color: #363636;
    border-bottom-left-radius: 0.2rem;
  }

  .chat-typing span {
    animation: blink 1.2s infinite;
    margin: 0 1px;
    font-size: 0.6rem;
  }
  .chat-typing span:nth-child(2) {
    animation-delay: 0.2s;
  }
  .chat-typing span:nth-child(3) {
    animation-delay: 0.4s;
  }
  @keyframes blink {
    0%,
    80%,
    100% {
      opacity: 0.2;
    }
    40% {
      opacity: 1;
    }
  }

  .chat-input-row {
    display: flex;
    gap: 0.4rem;
    padding: 0.5rem 0.6rem;
    border-top: 1px solid #ededed;
    flex-shrink: 0;
  }
  .chat-input-row .input {
    border-radius: 1rem;
  }
  .chat-input-row .button {
    border-radius: 1rem;
  }

  /* dark theme overrides */
  .chat-popup.dark {
    background: hsl(215, 18%, 13%);
    border-color: hsl(215, 18%, 27%);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.55);
  }
  .chat-popup.dark .chat-thread {
    background: hsl(215, 18%, 13%);
  }
  .chat-popup.dark .chat-bubble-assistant {
    background: hsl(215, 18%, 20%);
    color: hsl(0, 0%, 90%);
  }
  .chat-popup.dark .chat-hint {
    color: hsl(0, 0%, 48%);
  }
  .chat-popup.dark .chat-input-row {
    border-top-color: hsl(215, 18%, 27%);
  }
</style>
