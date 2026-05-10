import { describe, expect, test } from "bun:test";
import { applyRules } from "./spreadsheet";
import type { ImportRule } from "./utils";

describe("applyRules", () => {
  const rules: ImportRule[] = [
    { name: "Food delivery", match: "swiggy|zomato", account: "Expenses:Food:Delivery" },
    { name: "Salary", match: "salary", account: "Income:Salary" }
  ];

  test("matches first rule when pattern found anywhere in row", () => {
    const rows = [{ A: "SWIGGY ORDER 12345", B: "200", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Expenses:Food:Delivery");
  });

  test("matches second rule when first does not match", () => {
    const rows = [{ A: "Monthly salary credit", B: "50000", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Income:Salary");
  });

  test("sets empty string when no rule matches", () => {
    const rows = [{ A: "ATM withdrawal", B: "1000", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("");
  });

  test("first-match-wins when multiple rules could match", () => {
    const overlapping: ImportRule[] = [
      { name: "First", match: "credit", account: "Income:Other" },
      { name: "Second", match: "salary credit", account: "Income:Salary" }
    ];
    const rows = [{ A: "salary credit", B: "50000", index: 0 }];
    const result = applyRules(rows, overlapping);
    expect(result[0].ACCOUNT).toBe("Income:Other");
  });

  test("skips invalid regex without throwing", () => {
    const badRules: ImportRule[] = [
      { name: "Bad", match: "[invalid", account: "Expenses:Bad" }
    ];
    const rows = [{ A: "some text", B: "100", index: 0 }];
    const result = applyRules(rows, badRules);
    expect(result[0].ACCOUNT).toBe("");
  });

  test("matches against all columns concatenated", () => {
    const rows = [{ A: "2024-01-01", B: "REF123", C: "Swiggy", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Expenses:Food:Delivery");
  });

  test("preserves existing row fields", () => {
    const rows = [{ A: "some text", B: "100", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].A).toBe("some text");
    expect(result[0].B).toBe("100");
    expect(result[0].index).toBe(0);
  });
});
