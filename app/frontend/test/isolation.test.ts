import { expect, test } from 'bun:test';

test('each test file gets fresh modules, so state a module built against one file’s happy-dom window never reaches the next file', () => {
    const run = Bun.spawnSync(['bun', 'run', 'test', './test/isolation/first.ts', './test/isolation/second.ts'], {
        cwd: `${import.meta.dir}/..`,
        stderr: 'pipe',
    });
    expect(run.exitCode, run.stderr.toString()).toBe(0);
});
