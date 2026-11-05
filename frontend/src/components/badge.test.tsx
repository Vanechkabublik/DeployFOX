import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Badge } from "./badge";

describe("status badge", () => {
  it("keeps deployment status accessible without relying on color", () => {
    for (const status of ["SUCCESS", "FAILED", "QUEUED", "CANCELLED"]) {
      const html = renderToStaticMarkup(<Badge status={status} />);
      expect(html).toContain(status.toLowerCase());
      expect(html).not.toContain("undefined");
    }
  });
});
