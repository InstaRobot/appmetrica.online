import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { richText } from "./rich-text";

describe("richText", () => {
  it("puts nodes in place of the placeholders of a message", () => {
    const { container } = render(
      <p>
        {richText("Run {command} and open {link}.", { command: <code>cli</code>, link: "it" })}
      </p>,
    );

    expect(container.querySelector("p")?.innerHTML).toBe("Run <code>cli</code> and open it.");
  });

  it("keeps unknown placeholders as they are", () => {
    const { container } = render(<p>{richText("{a} and {b}", { a: <b>A</b> })}</p>);

    expect(container.textContent).toBe("A and {b}");
  });
});
