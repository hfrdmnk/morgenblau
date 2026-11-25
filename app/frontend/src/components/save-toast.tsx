import { useEffect, useRef, useState } from 'react';

import { CheckIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { Button } from '@/components/ui/button';
import { Kbd } from '@/components/ui/kbd';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useSaveState } from '@/hooks/use-entry-save';
import { useShortcuts } from '@/hooks/use-shortcuts';
import type { EntrySave } from '@/lib/entry-save';
import { saveState, withState } from '@/lib/library';

export function SaveToast({ save, onHold }: { save: EntrySave; onHold: (held: boolean) => void }) {
    const [open, setOpen] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    const { tags } = useSaveState(save);
    const archive = () => save.setTags(withState(tags, 'archived'));
    useShortcuts({ t: () => setOpen(true), 'Shift+A': archive });
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
                <ToastTitle archive={archive} archived={saveState(tags) === 'archived'} />
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

function ToastTitle({ archived, archive }: { archived: boolean; archive: () => void }) {
    return (
        <>
            <Frame grow>
                <Text as="span" weight="medium">
                    {archived ? 'Saved to Archive' : 'Saved to Library'}
                </Text>
            </Frame>
            {archived ? null : (
                <Button aria-keyshortcuts="Shift+A" onClick={archive} size="inline" variant="quiet">
                    Archive
                    <Kbd hideOnTouch>⇧A</Kbd>
                </Button>
            )}
        </>
    );
}
