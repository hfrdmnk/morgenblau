import { ArrowRightIcon } from '@/components/icons';
import { Link } from '@/components/ui/link';
import { Text } from '@/design/text';

export function BackLink({ href, label, keyShortcut }: { href: string; label: string; keyShortcut?: string }) {
    return (
        <Link aria-keyshortcuts={keyShortcut} href={href} variant="back">
            <ArrowRightIcon rotate={180} size="md" />
            <Text as="span">{label}</Text>
        </Link>
    );
}
