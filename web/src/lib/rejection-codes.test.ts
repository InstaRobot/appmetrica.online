import { describe, expect, it } from "vitest";
import { getMessages } from "@/messages";
import { rejectionCode } from "./rejection-codes";

// Семь RejectionCode из contracts/openapi/ingest.yaml и batch-коды ingest (14-ux-ui-spec.md §14.3).
const EVENT_CODES = [
  "missing_field",
  "invalid_field",
  "unknown_event_name",
  "event_too_old",
  "user_id_looks_like_pii",
  "duration_required",
  "install_rate_limited",
];
const BATCH_CODES = [
  "invalid_json",
  "invalid_batch",
  "unsupported_content_encoding",
  "payload_too_large",
  "unsupported_media_type",
  "app_archived",
];

const ru = getMessages("ru").integration.rejectionCodes;
const en = getMessages("en").integration.rejectionCodes;

describe("rejection codes", () => {
  it("describes every rejection code with a hint in both languages", () => {
    for (const texts of [ru, en]) {
      expect(Object.keys(texts).sort()).toEqual([...EVENT_CODES, ...BATCH_CODES].sort());
      for (const code of [...EVENT_CODES, ...BATCH_CODES]) {
        const text = rejectionCode(code, texts);
        expect(text.description, code).not.toBe("");
        expect(text.hint, code).not.toBe("");
      }
    }
    expect(rejectionCode("event_too_old", ru)).toEqual({
      description: "Событие старше 7 дней",
      hint: "Удаляйте из офлайн-очереди события старше 7 дней",
    });
    expect(rejectionCode("user_id_looks_like_pii", ru).description).toBe(
      "user_id похож на email или телефон",
    );
  });

  it("describes rejection codes in English", () => {
    expect(rejectionCode("event_too_old", en)).toEqual({
      description: "Event older than 7 days",
      hint: "Drop events older than 7 days from the offline queue",
    });
    expect(rejectionCode("user_id_looks_like_pii", en).description).toBe(
      "user_id looks like an email or a phone number",
    );
  });

  it("falls back to the raw code for unknown codes", () => {
    for (const texts of [ru, en]) {
      expect(rejectionCode("brand_new_code", texts)).toEqual({
        description: "brand_new_code",
        hint: null,
      });
      expect(rejectionCode("toString", texts)).toEqual({ description: "toString", hint: null });
    }
  });
});
