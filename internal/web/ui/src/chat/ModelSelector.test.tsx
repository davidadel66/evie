import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ModelSelector } from "./ModelSelector";

const models = [
  {id:"openai/gpt-test", name:"GPT Test", provider:"openai"},
  {id:"anthropic/claude-test", name:"Claude Test", provider:"anthropic"},
];

describe("chat model selector", () => {
  it("groups by provider and retains a configured model absent from the catalog", () => {
    const html = renderToStaticMarkup(<ModelSelector models={models} value="openai/gpt-test" onChange={() => {}} onRetry={() => {}} />);
    expect(html.indexOf('<optgroup label="Anthropic">')).toBeLessThan(html.indexOf('<optgroup label="OpenAI">'));
    expect(html).toContain('aria-label="Chat model"');
    expect(html).toContain('<optgroup label="Anthropic">');
    expect(html).toContain('<option value="openai/gpt-test" selected="">GPT Test</option>');
  });
  it("disables switching during a turn and shows an actionable error without losing selection", () => {
    const html = renderToStaticMarkup(<ModelSelector models={[]} value="deepseek/custom" disabled problem="Models unavailable" onChange={() => {}} onRetry={() => {}} />);
    expect(html).toContain('disabled=""');
    expect(html).toContain('value="deepseek/custom" selected=""');
    expect(html).toContain('role="alert"');
    expect(html).toContain('Retry');
  });
});
