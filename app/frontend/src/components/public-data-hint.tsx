import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useAppProfile } from '@/hooks/use-app-profile';

export function PublicDataHint({ centered = false }: { centered?: boolean }) {
    const hint = (
        <Text align={centered ? 'center' : undefined} size="xs" tone="subtle" wrap={centered ? 'balance' : 'pretty'}>
            Your data will be saved to your <PdsLink /> and be public (private data coming soon).
        </Text>
    );
    return centered ? <Frame paddingTop="2xs">{hint}</Frame> : hint;
}

export function PdsLink() {
    const profile = useAppProfile();
    if (profile.kind !== 'ready') return 'PDS';
    return (
        <Link href={`https://pds.ls/at://${profile.profile.did}`} rel="noopener noreferrer" target="_blank">
            PDS
        </Link>
    );
}
