/** How a text splits into SMS parts. An estimate: the phone does the real split. */
export interface SegmentInfo {
  /** `GSM-7` when every character is in the GSM 03.38 alphabet, otherwise `UCS-2`. */
  encoding: "GSM-7" | "UCS-2";
  /** Length in encoding units: septets for GSM-7, UTF-16 code units for UCS-2. */
  length: number;
  /** Number of SMS parts the carrier will count. 0 for an empty text. */
  segments: number;
  /** Units per part at this length: 160 or 153 for GSM-7, 70 or 67 for UCS-2. */
  per_segment: number;
  /** Units that still fit before another part is needed. */
  remaining: number;
}

// GSM 03.38 basic character set. Each costs one septet.
const GSM_BASIC = new Set(
  "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà",
);

// GSM 03.38 extension table. Each costs two septets (escape plus character).
const GSM_EXTENDED = new Set("\f^{}\\[~]|€");

/**
 * Estimates how many SMS parts `text` needs and which encoding applies.
 * Useful for a counter under a text box or for cost estimates.
 */
export function countSegments(text: string): SegmentInfo {
  let septets = 0;
  let gsm = true;
  for (const ch of text) {
    if (GSM_BASIC.has(ch)) {
      septets += 1;
    } else if (GSM_EXTENDED.has(ch)) {
      septets += 2;
    } else {
      gsm = false;
      break;
    }
  }
  const length = gsm ? septets : text.length;
  const single = gsm ? 160 : 70;
  const multi = gsm ? 153 : 67;
  const per_segment = length <= single ? single : multi;
  const segments = length === 0 ? 0 : Math.ceil(length / per_segment);
  const remaining = segments === 0 ? single : segments * per_segment - length;
  return { encoding: gsm ? "GSM-7" : "UCS-2", length, segments, per_segment, remaining };
}
