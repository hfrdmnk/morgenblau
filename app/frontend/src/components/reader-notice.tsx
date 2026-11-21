import { Text } from '@/design/text';

export function ReaderNotice({ notice }: { notice: string }) {
    // The live region must exist before its message arrives, or screen readers skip the announcement.
    return (
        <Text role="status" size="sm" tone="muted" visuallyHidden={!notice || undefined}>
            {notice}
        </Text>
    );
}
