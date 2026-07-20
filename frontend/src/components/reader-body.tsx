import DOMPurify from 'dompurify';
import { useMemo } from 'react';

// Body is server-sanitized (bluemonday UGC) at ingest; DOMPurify is defense-in-depth against a server regression.
export function ReaderBody({ html }: { html: string }) {
    const clean = useMemo(() => DOMPurify.sanitize(html), [html]);
    return (
        <div
            className="font-serif text-base leading-relaxed text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:italic [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:font-sans [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:font-sans [&_img]:rounded-2xl [&_ol]:mb-5 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:mb-5 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-muted [&_pre]:p-4 [&_pre]:font-mono [&_pre]:text-sm [&_ul]:mb-5 [&_ul]:list-disc [&_ul]:pl-6"
            dangerouslySetInnerHTML={{ __html: clean }}
        />
    );
}
