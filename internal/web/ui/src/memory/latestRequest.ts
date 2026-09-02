export class LatestMemoryRequest {
  private generation = 0;

  invalidate() {
    this.generation += 1;
  }

  async run<T>(
    request: () => Promise<T>,
    commit: (value: T) => void,
    reject: (error: unknown) => void,
  ) {
    const generation = ++this.generation;
    try {
      const value = await request();
      if (generation === this.generation) commit(value);
    } catch (error) {
      if (generation === this.generation) reject(error);
    }
  }
}
