import { createContext, useContext, useLayoutEffect, useRef, type ReactNode } from 'react';

type SkeletonProps<T> = {
    data?: T | null;
    sample?: T;
    error?: ReactNode;
    label: string;
    children: ReactNode | ((data: T) => ReactNode);
};

export function Skeleton<T>({ data, sample, error, label, children }: SkeletonProps<T>) {
    if (error) return error;
    if (typeof children !== 'function') return <Placeholder label={label}>{children}</Placeholder>;
    if (data != null) return <div>{children(data)}</div>;
    return <Placeholder label={label}>{sample === undefined ? null : children(sample)}</Placeholder>;
}

const InSample = createContext(false);

let showing = 0;
let startedAt: number | null = null;

function Placeholder({ label, children }: { label: string; children: ReactNode }) {
    const nested = useContext(InSample);
    const source = useRef<HTMLDivElement>(null);
    const copy = useRef<HTMLDivElement>(null);
    useLayoutEffect(() => {
        if (nested) return;
        startedAt ??= performance.now();
        showing += 1;
        copy.current!.style.setProperty('--skeleton-elapsed', `${performance.now() - startedAt}ms`);
        const update = () => copy.current!.replaceChildren(barred(source.current!));
        update();
        const observer = new MutationObserver(update);
        observer.observe(source.current!, { attributes: true, characterData: true, childList: true, subtree: true });
        return () => {
            observer.disconnect();
            showing -= 1;
            queueMicrotask(() => {
                if (showing === 0) startedAt = null;
            });
        };
    }, [nested]);
    if (nested) return <div>{children}</div>;
    return (
        <div aria-busy="true">
            <span className="sr-only">{label}</span>
            <div aria-hidden="true" inert>
                <div hidden ref={source}>
                    <InSample.Provider value>{children}</InSample.Provider>
                </div>
                <div className="skeleton" ref={copy} />
            </div>
        </div>
    );
}

function barred(source: HTMLElement) {
    const range = document.createRange();
    range.selectNodeContents(source);
    const copy = range.cloneContents();
    for (const element of copy.querySelectorAll('[id]')) element.removeAttribute('id');
    const walker = document.createTreeWalker(copy, NodeFilter.SHOW_TEXT);
    const texts: Text[] = [];
    while (walker.nextNode()) {
        const node = walker.currentNode;
        if (node instanceof Text && node.data.trim() && !node.parentElement?.closest('svg')) texts.push(node);
    }
    for (const text of texts) {
        const bar = document.createElement('span');
        bar.className = 'skeleton-text';
        text.replaceWith(bar);
        bar.append(text);
    }
    return copy;
}
