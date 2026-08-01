export type TruthStatus = "computed" | "confirmed" | "corrected" | "self_reported";
export type TruthChannel = "sms" | "pdf";

export function channelLabel(channel: TruthChannel | undefined): string {
  return channel === "pdf" ? "statement PDF" : "SMS";
}

export function badgeClass(status: TruthStatus): string {
  if (status === "corrected") return "is-warning is-light";
  if (status === "self_reported") return "is-info is-light";
  return "is-success is-light";
}

export function badgeIcon(status: TruthStatus): string {
  if (status === "corrected") return "fa-pen";
  if (status === "self_reported") return "fa-user-check";
  return "fa-check";
}
