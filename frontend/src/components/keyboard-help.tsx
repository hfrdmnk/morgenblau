import { useState } from 'react';

import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import { useKeyboard } from '@/hooks/use-keyboard';

type Shortcut = { keys: string[]; label: string };
type Group = { title: string; shortcuts: Shortcut[] };

const GROUPS: Group[] = [
    {
        title: 'Reader',
        shortcuts: [
            { keys: ['Esc'], label: 'Close reader' },
            { keys: ['B'], label: 'Save or unsave' },
            { keys: ['O'], label: 'Open original article' },
            { keys: ['M'], label: 'Toggle reader text' },
        ],
    },
    {
        title: 'Lists',
        shortcuts: [
            { keys: ['↑', '↓'], label: 'Move selection' },
            { keys: ['↵'], label: 'Open entry' },
            { keys: ['Esc'], label: 'Clear selection' },
        ],
    },
    {
        title: 'Digest',
        shortcuts: [
            { keys: ['←', '→'], label: 'Previous or next day' },
            { keys: ['T'], label: 'Jump to today' },
            { keys: ['R'], label: 'Refresh' },
        ],
    },
    {
        title: 'Anywhere',
        shortcuts: [{ keys: ['?'], label: 'Show this help' }],
    },
];

export function KeyboardHelp() {
    const [open, setOpen] = useState(false);

    useKeyboard({ '?': () => setOpen(true) });

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Keyboard shortcuts</DialogTitle>
                </DialogHeader>
                <div className="flex flex-col gap-5">
                    {GROUPS.map((group) => (
                        <section
                            key={group.title}
                            className="flex flex-col gap-2"
                        >
                            <h3 className="text-[0.6875rem] font-light tracking-wide text-muted-foreground uppercase">
                                {group.title}
                            </h3>
                            <dl className="flex flex-col gap-1.5">
                                {group.shortcuts.map((shortcut) => (
                                    <div
                                        key={shortcut.label}
                                        className="flex items-center justify-between gap-4"
                                    >
                                        <dt className="text-foreground">
                                            {shortcut.label}
                                        </dt>
                                        <dd className="flex items-center gap-1">
                                            {shortcut.keys.map((key) => (
                                                <Kbd key={key}>{key}</Kbd>
                                            ))}
                                        </dd>
                                    </div>
                                ))}
                            </dl>
                        </section>
                    ))}
                </div>
            </DialogContent>
        </Dialog>
    );
}

function Kbd({ children }: { children: string }) {
    return (
        <kbd className="inline-flex h-6 min-w-6 items-center justify-center rounded-sm bg-overlay-2 px-1.5 font-sans text-xs font-normal text-muted-foreground">
            {children}
        </kbd>
    );
}
