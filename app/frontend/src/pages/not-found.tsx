import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';

export function NotFound() {
    useDocumentTitle('Page not found');
    return (
        <Frame as="main" id="main" maxWidth="3xl" paddingX={{ base: 'xl', sm: '3xl' }} width="full">
            <Text as="h1" visuallyHidden>
                Page not found
            </Text>
            <EmptyState
                symbol="loop"
                title="Page not found"
                description="This address doesn’t lead to anything in Morgenblau."
                action={
                    <Button
                        render={<Link href={PATHS.digest} variant="plain" />}
                        nativeButton={false}
                        variant="secondary"
                    >
                        Go to the digest
                    </Button>
                }
            />
        </Frame>
    );
}
