import { persisted } from "svelte-local-storage-store";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  text: string;
  ts: number;
}

const MAX_MESSAGES = 50;

export function generateId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2);
}

export const chatMessages = persisted<ChatMessage[]>("paisa:chat:history", []);

export function clearHistory(): void {
  chatMessages.set([]);
}

export function appendMessage(msg: ChatMessage): void {
  chatMessages.update((msgs) => [...msgs, msg].slice(-MAX_MESSAGES));
}
