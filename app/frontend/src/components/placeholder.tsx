type PlaceholderProps = { label: string };

export function Placeholder({ label }: PlaceholderProps) {
    return (
        <main className="page-placeholder">
            <span>{label}</span>
        </main>
    );
}
