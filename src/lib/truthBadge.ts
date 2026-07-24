export type TruthStatus = "computed" | "confirmed" | "corrected";
export type TruthChannel = "sms" | "pdf";

export function channelLabel(channel: TruthChannel | undefined): string {
  return channel === "pdf" ? "statement PDF" : "SMS";
}

export function badgeClass(status: TruthStatus): string {
  return status === "corrected" ? "is-warning is-light" : "is-success is-light";
}

export function badgeIcon(status: TruthStatus): string {
  return status === "corrected" ? "fa-pen" : "fa-check";
}
