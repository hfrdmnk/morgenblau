type Listener = () => void;

const listeners = new Set<Listener>();

// Saving changes a list the reader doesn't own, so it announces instead of patching it.
export function emitLibraryMutation(): void {
    for (const listener of listeners) {
        listener();
    }
}

export function subscribeLibraryMutation(listener: Listener): () => void {
    listeners.add(listener);
    return () => {
        listeners.delete(listener);
    };
}
