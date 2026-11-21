import { PagePlaceholder } from '@/components/ui/app-chrome';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';

type PlaceholderProps = { label: string };

export function Placeholder({ label }: PlaceholderProps) {
    useDocumentTitle(label);
    return (
        <PagePlaceholder id="main">
            <Text as="h1">{label}</Text>
        </PagePlaceholder>
    );
}
