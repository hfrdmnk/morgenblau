import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';

GlobalRegistrator.register({ url: 'http://app.example.com/' });
const { act, useRef } = await import('react');
const { createRoot } = await import('react-dom/client');
const { PLAYER_ORIGIN, useYouTubePlayer } = await import('./use-youtube-player');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

afterAll(() => GlobalRegistrator.unregister());

function Player() {
    const frame = useRef<HTMLIFrameElement>(null);
    const player = useYouTubePlayer(frame);
    return <><iframe ref={frame} title="Example video" /><button onClick={() => player.seekTo(83_000)}>Play from 1:23</button><output>{`${player.ready} ${player.playing} ${player.currentTime}`}</output></>;
}

function deliver(source: unknown, origin: string, data: object) {
    window.dispatchEvent(new MessageEvent('message', { data: JSON.stringify(data), origin, source: source as Window }));
}

test('the player reports time and state only from its own embed, and seeking posts commands to it', async () => {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    try {
        await act(async () => root.render(<Player />));
        const frame = container.querySelector('iframe')!;
        const sent: Array<[string, string]> = [];
        frame.contentWindow!.postMessage = ((message: string, target: string) => sent.push([message, target])) as Window['postMessage'];
        const state = () => container.querySelector('output')!.textContent;
        expect(state()).toBe('false false 0');

        const info = { event: 'infoDelivery', info: { currentTime: 12.5, playerState: 1 } };
        await act(async () => deliver(frame.contentWindow, 'https://www.youtube.com', info));
        await act(async () => deliver(window, PLAYER_ORIGIN, info));
        expect(state()).toBe('false false 0');

        await act(async () => deliver(frame.contentWindow, PLAYER_ORIGIN, info));
        expect(state()).toBe('true true 12500');
        await act(async () => deliver(frame.contentWindow, PLAYER_ORIGIN, { event: 'onStateChange', info: 2 }));
        expect(state()).toBe('true false 12500');

        await act(async () => container.querySelector('button')!.click());
        const commands = sent.map(([message, target]) => {
            expect(target).toBe(PLAYER_ORIGIN);
            return JSON.parse(message);
        }).filter((m) => m.event === 'command' && m.func !== 'addEventListener');
        expect(commands.map((m) => [m.func, m.args])).toEqual([['seekTo', [83, true]], ['playVideo', []]]);
        expect(state()).toBe('true false 83000');
    } finally {
        act(() => root.unmount());
        container.remove();
    }
});
