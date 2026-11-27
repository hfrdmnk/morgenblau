import { Text } from '@/design/text';

export function ReaderNotice({ notice }: { notice: string }) {
    return (
        <Text role="status" size="sm" tone="muted" visuallyHidden={!notice || undefined}>
            {notice}
        </Text>
    );
}
