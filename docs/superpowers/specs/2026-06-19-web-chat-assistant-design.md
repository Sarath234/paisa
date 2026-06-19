# Web Finance Assistant Design Spec

## Goal

Add a Q&A chatbot to the web UI that reuses the existing `qa` package (Ollama → structured query → paisa API → formatted reply), surfaced in two places: a dedicated `/assistant` page in the sidebar nav, and a floating popup widget accessible from every page.

## Architecture

```
Browser
  ├── ChatWidget.svelte  (FAB + popup, mounted in +layout.svelte)
  ├── /assistant page    (full thread, nav icon)
  └── chat store         (shared localStorage, capped at 50 messages)
        ↓ POST /api/agent/chat
Main paisa server  (proxy, same pattern as /api/agent/parse)
        ↓ POST /chat
paisa-agent  (qa.Extract → Ollama → qa.Answerer → paisa API)
```

The `/chat` endpoint in `paisa-agent` is a thin wrapper around the existing `qa.Extract` + `qa.Answerer` pipeline — no new packages required.

## Backend

### `cmd/paisa-agent/main.go`

Add a `/chat` handler inside the existing `serveHTTP` function:

```go
mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var req struct {
        Message string `json:"message"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
        return
    }
    q, err := qa.Extract(req.Message, cfg.Ollama)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnprocessableEntity)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    answer, err := answerer.Answer(q)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{"reply": answer})
})
```

`answerer` is a `*qa.Answerer` constructed once in `main()`:

```go
answerer := &qa.Answerer{
    Client: paisaclient.New(cfg.Paisa.URL),
    Now:    time.Now,
}
```

### `internal/server/agent.go`

Add:

```go
const agentChatURL = "http://127.0.0.1:7501/chat"

func ChatWithAgent(c *gin.Context) {
    proxyToAgent(c, agentChatURL)
}
```

### `internal/server/server.go`

Register before `router.NoRoute`:

```go
router.POST("/api/agent/chat", func(c *gin.Context) { ChatWithAgent(c) })
```

## Frontend

### `src/lib/stores/chat.ts`

Svelte writable store backed by `localStorage`. Exported as `chatMessages`.

```typescript
export interface ChatMessage {
  id: string;       // crypto.randomUUID()
  role: "user" | "assistant";
  text: string;
  ts: number;       // Date.now()
}

const STORAGE_KEY = "paisa:chat:history";
const MAX_MESSAGES = 50;
```

On init: read from `localStorage`, parse JSON, fall back to `[]`.  
On update: write back to `localStorage`, trim to last 50 messages.  
Export a `clearHistory()` helper that sets the store to `[]` and clears the key.

### `src/lib/utils.ts`

Add ajax overload:

```typescript
export function ajax(
  route: "/api/agent/chat",
  options?: RequestOptions
): Promise<{ reply?: string; error?: string }>;
```

### `src/lib/components/ChatWidget.svelte`

Floating FAB + popup bubble. State:

```typescript
let open = false;          // popup visible
let input = "";
let loading = false;
```

**FAB:** `position: fixed; bottom: 1.5rem; right: 1.5rem; z-index: 40` — blue circle with 💬 icon, toggles `open`.

**Popup:** appears above the FAB when `open`, `width: 360px`, `max-height: 480px`. Structure:
- Header bar (blue): "💬 Assistant" | `↗` (navigate to `/assistant`) | `✕` (close)
- Message thread: last 8 messages from `chatMessages` store, scrolled to bottom on new message
- Input bar: text input + send button; Enter submits

On send:
1. Append user message to store
2. Set `loading = true`, show typing dots as last bubble
3. POST `{"message": input}` to `/api/agent/chat`
4. On success: append assistant reply to store
5. On error: append error message as assistant bubble ("⚠️ …")
6. Set `loading = false`

**Error messages:**
- 502 (agent unreachable): "⚠️ paisa-agent is not running. Start it to use the assistant."
- 422 (extract failed): "⚠️ Couldn't understand that — try rephrasing, e.g. \"food spend this month\""
- 500 (answerer failed): "⚠️ Paisa server unreachable — is paisa running?"

### `src/routes/(app)/assistant/+page.svelte`

Full-page chat thread. Same send logic as `ChatWidget` but shows all 50 messages, fully scrollable.

Header: "Finance Assistant" title, "Clear history" button (calls `clearHistory()`).  
Input bar at the bottom, pinned.  
Auto-scroll to bottom on new message via `afterUpdate`.

### `src/routes/(app)/+layout.svelte`

Two changes:

1. Import and mount `<ChatWidget />` (renders the FAB + popup on every page).
2. Add the assistant nav icon in the sidebar, between search and settings:

```svelte
<NavIcon href="/assistant" icon="fa-comment-dots" label="Assistant" />
```

## Data Flow

```
User types question → ChatWidget appends user message → POST /api/agent/chat
  → paisa server proxies to paisa-agent:7501/chat
  → qa.Extract (1 Ollama call) → Query{intent, category, account, period}
  → qa.Answerer.Answer → calls /api/expenses or /api/networth etc on paisa
  → formatted string → {"reply": "..."}
  → ChatWidget appends assistant message → localStorage updated
```

Round-trip time depends on Ollama (typically 1–3 s on local hardware). The typing indicator covers this wait.

## Error Handling

All errors surface as assistant messages in the thread — never as alerts or toasts. This keeps the conversation flow intact and lets the user retry immediately.

If `paisa-agent` is not running, the 502 message tells the user exactly what to do. The rest of the app (ledger, dashboard) is unaffected.

## Files

**Create:**
- `src/lib/stores/chat.ts`
- `src/lib/components/ChatWidget.svelte`
- `src/routes/(app)/assistant/+page.svelte`

**Modify:**
- `cmd/paisa-agent/main.go` — add `/chat` handler, construct `answerer`
- `internal/server/agent.go` — add `ChatWithAgent`, `agentChatURL`
- `internal/server/server.go` — register `/api/agent/chat`
- `src/lib/utils.ts` — add ajax overload for `/api/agent/chat`
- `src/routes/(app)/+layout.svelte` — mount `ChatWidget`, add nav icon

## Out of Scope

- Streaming responses (full reply arrives at once, matching Telegram behavior)
- Multi-turn context (each question is independent; the LLM does not see prior messages)
- Auth / per-user history (paisa is single-user; localStorage is sufficient)
- New Q&A intents (reuses the four existing: `expense_summary`, `networth`, `account_balance`, `budget_status`)
