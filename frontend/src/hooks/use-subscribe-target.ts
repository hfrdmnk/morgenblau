import { useState } from 'react';

// Subscribe-dialog bookkeeping shared by the discover panels: which source the dialog targets,
// whether it's open, and the keys subscribed this session. Form state lives in useSubscribeDialog.
export function useSubscribeTarget<T extends { key: string }>() {
    const [dialogSource, setDialogSource] = useState<T | null>(null);
    const [dialogOpen, setDialogOpen] = useState(false);
    const [subscribedKeys, setSubscribedKeys] = useState<ReadonlySet<string>>(
        () => new Set(),
    );

    const onSubscribe = (source: T) => {
        setDialogSource(source);
        setDialogOpen(true);
    };

    // Called once the dialog's POST succeeds: mark the source subscribed (it stays in the list) and close.
    const onSubscribed = () => {
        if (dialogSource) {
            setSubscribedKeys((prev) => new Set(prev).add(dialogSource.key));
        }
        setDialogOpen(false);
    };

    return {
        dialogSource,
        dialogOpen,
        onDialogOpenChange: setDialogOpen,
        onSubscribe,
        onSubscribed,
        subscribedKeys,
    };
}
