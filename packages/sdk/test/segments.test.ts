import { describe, expect, it } from "vitest";
import { countSegments } from "../src/segments";

describe("countSegments", () => {
  it("treats an empty text as zero parts", () => {
    expect(countSegments("")).toEqual({ encoding: "GSM-7", length: 0, segments: 0, per_segment: 160, remaining: 160 });
  });

  it("counts plain text as GSM-7", () => {
    expect(countSegments("Hello")).toEqual({ encoding: "GSM-7", length: 5, segments: 1, per_segment: 160, remaining: 155 });
    expect(countSegments("a".repeat(160)).segments).toBe(1);
  });

  it("switches to 153 per part when concatenated", () => {
    const info = countSegments("a".repeat(161));
    expect(info).toEqual({ encoding: "GSM-7", length: 161, segments: 2, per_segment: 153, remaining: 145 });
    expect(countSegments("a".repeat(306)).segments).toBe(2);
    expect(countSegments("a".repeat(307)).segments).toBe(3);
  });

  it("charges two septets for extension characters", () => {
    expect(countSegments("€").length).toBe(2);
    expect(countSegments("[]{}").length).toBe(8);
    expect(countSegments("a".repeat(158) + "€").segments).toBe(1);
    expect(countSegments("a".repeat(159) + "€").segments).toBe(2);
  });

  it("keeps GSM-7 for accented letters in the alphabet", () => {
    expect(countSegments("Voilà, señor? Grüße! Ça marche.").encoding).toBe("GSM-7");
    // Lowercase c-cedilla is not in the basic table, unlike its uppercase form.
    expect(countSegments("ça").encoding).toBe("UCS-2");
  });

  it("falls back to UCS-2 for other characters", () => {
    expect(countSegments("Merhaba dünya ş")).toEqual({ encoding: "UCS-2", length: 15, segments: 1, per_segment: 70, remaining: 55 });
    expect(countSegments("ş".repeat(70)).segments).toBe(1);
    expect(countSegments("ş".repeat(71))).toEqual({ encoding: "UCS-2", length: 71, segments: 2, per_segment: 67, remaining: 63 });
  });

  it("counts astral characters as two UCS-2 units", () => {
    expect(countSegments("😀")).toEqual({ encoding: "UCS-2", length: 2, segments: 1, per_segment: 70, remaining: 68 });
  });
});
