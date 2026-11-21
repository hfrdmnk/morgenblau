// A source's favicon, sized like an icon; one that fails to load hides instead of showing a broken image.
function SourceIcon({ src, size = 'md' }: { src?: string | null; size?: 'md' | '2xl' }) {
    if (!src) return null;
    return (
        <img
            alt=""
            data-slot="source-icon"
            className={
                size === '2xl' ? 'size-6 shrink-0 rounded-sm object-cover' : 'size-4 shrink-0 rounded-sm object-cover'
            }
            loading="lazy"
            onError={(event) => {
                event.currentTarget.hidden = true;
            }}
            referrerPolicy="no-referrer"
            src={src}
        />
    );
}

export { SourceIcon };
