import { describe, expect, it } from "vitest";
import { LatestMemoryRequest } from "./latestRequest";

describe("LatestMemoryRequest", () => {
  it("never commits a delayed response after a newer scope request", async () => {
    const requests = new LatestMemoryRequest();
    const first = deferred<string>();
    const second = deferred<string>();
    const committed: string[] = [];
    const rejected: unknown[] = [];

    const oldScope = requests.run(() => first.promise, (value) => committed.push(value), (error) => rejected.push(error));
    const selectedScope = requests.run(() => second.promise, (value) => committed.push(value), (error) => rejected.push(error));
    second.resolve("global");
    await selectedScope;
    first.resolve("workspace:sibling");
    await oldScope;

    expect(committed).toEqual(["global"]);
    expect(rejected).toEqual([]);
  });

  it("drops a pending detail response when its listing is invalidated", async () => {
    const requests = new LatestMemoryRequest();
    const detail = deferred<string>();
    const committed: string[] = [];
    const pending = requests.run(() => detail.promise, (value) => committed.push(value), () => undefined);

    requests.invalidate();
    detail.resolve("claim from the prior scope");
    await pending;

    expect(committed).toEqual([]);
  });

  it("drops an old-page detail when a pending list commits a new page", async () => {
    const lists = new LatestMemoryRequest();
    const details = new LatestMemoryRequest();
    const nextPage = deferred<string>();
    const oldPageDetail = deferred<string>();
    const committed: string[] = [];
    const listing = lists.run(
      () => nextPage.promise,
      (value) => {
        details.invalidate();
        committed.push(value);
      },
      () => undefined,
    );
    const detail = details.run(() => oldPageDetail.promise, (value) => committed.push(value), () => undefined);

    nextPage.resolve("page 2");
    await listing;
    oldPageDetail.resolve("page 1 detail");
    await detail;

    expect(committed).toEqual(["page 2"]);
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((accept, decline) => {
    resolve = accept;
    reject = decline;
  });
  return { promise, resolve, reject };
}
