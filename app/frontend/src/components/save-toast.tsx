import { useEffect, useRef, useState } from 'react';

import { CheckIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { Button } from '@/components/ui/button';
import { Kbd } from '@/components/ui/kbd';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useShortcuts } from '@/hooks/use-shortcuts';
import type { EntrySave } from '@/lib/entry-save';

export function SaveToast({ save, onHold }: { save: EntrySave; onHold: (held: boolean) => void }) {
    const [open, setOpen] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    useShortcuts({ t: () => setOpen(true) });
    useEffect(() => {
        if (open) input.current?.focus();
    }, [open]);

    return (
        <Frame
            gap="m"
            minWidth={0}
            onBlur={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget)) onHold(false);
            }}
            onFocus={() => onHold(true)}
            width="full"
        >
            <Frame align="center" direction="row" gap="m">
                <CheckIcon size="md" tone="success-icon" />
                <Frame grow>
                    <Text as="span" weight="medium">
                        Saved to Library
                    </Text>
                </Frame>
                {!open ? (
                    <Button aria-keyshortcuts="t" onClick={() => setOpen(true)} size="inline" variant="quiet">
                        Add tags
                        <Kbd hideOnTouch>T</Kbd>
                    </Button>
                ) : null}
            </Frame>
            {open ? <SaveTagEditor inputRef={input} save={save} /> : null}
        </Frame>
    );
}
