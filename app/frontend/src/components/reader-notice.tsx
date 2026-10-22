export function ReaderNotice({ notice }: { notice: string }) {
    return notice ? <p className="mt-5 text-sm text-muted-foreground" role="status">{notice}</p> : null;
}
