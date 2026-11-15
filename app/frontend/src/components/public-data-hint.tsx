import { Link } from '@/components/ui/link';
import { useAppProfile } from '@/hooks/use-app-profile';
import { cn } from '@/lib/utils';

export function PublicDataHint({ className }: { className?: string }) {
    return (
        <p className={cn('text-xs text-pretty text-subtle-foreground', className)}>
            Your data will be saved to your <PdsLink /> and be public (private data coming soon).
        </p>
    );
}

export function PdsLink() {
    const profile = useAppProfile();
    if (profile.kind !== 'ready') return 'PDS';
    return <Link href={`https://pds.ls/at://${profile.profile.did}`} rel="noopener noreferrer" target="_blank">PDS</Link>;
}
