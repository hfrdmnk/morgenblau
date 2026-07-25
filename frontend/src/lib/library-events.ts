type LibraryMutation = { kind: 'save' | 'share' };

type Listener = (event: LibraryMutation) => void;

const listeners = new Set<Listener>();

// Saving or sharing changes lists the mutating surface doesn't own, so it announces instead of patching them.
export function emitLibraryMutation(event: LibraryMutation): void {
    for (const listener of listeners) {
        listener(event);
    }
}

export function subscribeLibraryMutation(listener: Listener): () => void {
    listeners.add(listener);
    return () => {
        listeners.delete(listener);
    };
}
