// Motion binds its frame loop to the requestAnimationFrame it sees on first import, and every test file shares that
// import. A frame scheduled on a test file's happy-dom timers dies with that window and leaves motion waiting forever,
// so frames run on Bun's own timer, captured here before any file registers happy-dom.
const nativeSetTimeout = setTimeout;
globalThis.requestAnimationFrame = (callback) => Number(nativeSetTimeout(() => callback(performance.now()), 16));
await import('motion/react');
