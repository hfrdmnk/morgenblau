export function ReaderNotice({ notice }: { notice: string }) {
    // The live region must exist before its message arrives, or screen readers skip the announcement.
    return <p className={notice ? 'mt-5 text-sm text-muted-foreground' : 'sr-only'} role="status">{notice}</p>;
}
