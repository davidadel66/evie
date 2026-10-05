import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ModelPicker, ModelProblem } from "./ModelSelector";

const models = [
  {id:"openai/gpt-test", name:"GPT Test", provider:"openai"},
  {id:"anthropic/claude-test", name:"Claude Test", provider:"anthropic"},
];
const base = {models, onChange: () => {}, onClose: () => {}};

describe("chat model picker", () => {
  it("groups by provider, marks the current model and retains one absent from the catalog", () => {
    const html = renderToStaticMarkup(<ModelPicker {...base} value="deepseek/custom" />);
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-label="Search models"');
    expect(html.indexOf('aria-label="Anthropic"')).toBeLessThan(html.indexOf('aria-label="OpenAI"'));
    expect(html).toContain('aria-label="DeepSeek"');
    expect(html).toMatch(/aria-current="true"[^>]*>.*deepseek\/custom/);
    expect(html).not.toContain("once the current reply finishes");
  });

  it("explains and disables switching during a turn", () => {
    const html = renderToStaticMarkup(<ModelPicker {...base} value="openai/gpt-test" disabled />);
    expect(html).toContain("The model can change once the current reply finishes.");
    expect(html.match(/data-model="true" disabled=""/g)).toHaveLength(2);
  });

  it("reports selection problems with Retry", () => {
    const html = renderToStaticMarkup(<ModelProblem problem="Models unavailable" onRetry={() => {}} />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("Retry");
    expect(renderToStaticMarkup(<ModelProblem onRetry={() => {}} />)).toBe("");
  });
});
