import { describe, expect, test } from "bun:test";
import { channelLabel, badgeClass, badgeIcon } from "./truthBadge";

describe("truthBadge", () => {
  test("channelLabel names pdf explicitly", () => {
    expect(channelLabel("pdf")).toBe("statement PDF");
  });

  test("channelLabel defaults to SMS for sms or undefined", () => {
    expect(channelLabel("sms")).toBe("SMS");
    expect(channelLabel(undefined)).toBe("SMS");
  });

  test("badgeClass is warning for corrected, success for confirmed", () => {
    expect(badgeClass("corrected")).toBe("is-warning is-light");
    expect(badgeClass("confirmed")).toBe("is-success is-light");
  });

  test("badgeIcon is pen for corrected, check for confirmed", () => {
    expect(badgeIcon("corrected")).toBe("fa-pen");
    expect(badgeIcon("confirmed")).toBe("fa-check");
  });

  test("badgeClass is info for self_reported", () => {
    expect(badgeClass("self_reported")).toBe("is-info is-light");
  });

  test("badgeIcon is a user-check icon for self_reported", () => {
    expect(badgeIcon("self_reported")).toBe("fa-user-check");
  });
});
