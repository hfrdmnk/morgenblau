const tones = { destructive: 'bg-destructive', warning: 'bg-warning' };

// An 8px dot that marks a state next to text; its parent says what the dot means.
function StatusDot({ tone }: { tone: keyof typeof tones }) {
    return <span aria-hidden="true" data-slot="status-dot" className={`inline-block size-2 shrink-0 rounded-full ${tones[tone]}`} />;
}

export { StatusDot };
