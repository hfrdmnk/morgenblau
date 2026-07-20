import { Button } from '@/components/ui/button';

export function DiscoverLoadMoreFooter({
    loading,
    onLoadMore,
}: {
    loading: boolean;
    onLoadMore: () => void;
}) {
    return (
        <div className="flex justify-center">
            <Button
                variant="ghost"
                size="sm"
                onClick={onLoadMore}
                disabled={loading}
            >
                {loading ? 'Loading…' : 'Load more'}
            </Button>
        </div>
    );
}
