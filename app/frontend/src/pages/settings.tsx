import { NewsletterAddress } from '@/components/newsletter-address';
import { Link } from '@/components/ui/link';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { readerFontLabels, readerFonts, useReaderFont } from '@/hooks/use-reader-font';
import { DataSettings } from '@/layouts/data-settings';
import { cn } from '@/lib/utils';

export function Settings() {
    const [readerFont, setReaderFont, fontError] = useReaderFont();
    useDocumentTitle('General settings');

    return (
        <DataSettings>
            <h1 className="text-2xl font-medium">General settings</h1>
            <div className="mt-12 max-w-xl space-y-12">
                <fieldset>
                    <legend className="text-lg font-medium">Reader font</legend>
                    {fontError ? (
                        <p role="alert" className="mt-1 text-sm text-destructive">{fontError}</p>
                    ) : (
                        <p className="mt-1 text-sm text-muted-foreground">Saved on this device.</p>
                    )}
                    <div className="mt-5 grid grid-cols-3 gap-3">
                        {readerFonts.map((font) => (
                            <label key={font} className="relative cursor-pointer select-none">
                                <input
                                    type="radio"
                                    name="reader-font"
                                    value={font}
                                    checked={readerFont === font}
                                    onChange={() => setReaderFont(font)}
                                    className="peer sr-only"
                                />
                                <span className={cn(
                                    'flex flex-col gap-5 rounded-xl p-5 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-ring forced-colors:border-2 forced-colors:border-transparent forced-colors:peer-checked:border-[Highlight]',
                                    readerFont === font ? 'bg-accent text-accent-foreground' : 'bg-muted text-foreground',
                                )}>
                                    <span aria-hidden="true" className={cn('truncate text-3xl', { sans: 'font-sans', serif: 'font-serif', mono: 'font-duo' }[font])}>
                                        Aa Bb Cc Dd Ee Ff Gg Hh Ii Jj Kk Ll Mm Nn Oo Pp Qq Rr Ss Tt Uu Vv Ww Xx Yy Zz
                                    </span>
                                    <span className="text-sm font-medium">{readerFontLabels[font]}</span>
                                </span>
                            </label>
                        ))}
                    </div>
                    <p className="mt-3 text-xs text-muted-foreground">
                        Geist is designed by <Link href="https://vercel.com" rel="noopener noreferrer" target="_blank">Vercel<span className="sr-only"> (opens in a new tab)</span></Link>,
                        Newsreader by <Link href="https://productiontype.com" rel="noopener noreferrer" target="_blank">Production Type<span className="sr-only"> (opens in a new tab)</span></Link> and
                        iA Writer Duo by <Link href="https://ia.net" rel="noopener noreferrer" target="_blank">Information Architects<span className="sr-only"> (opens in a new tab)</span></Link>.
                    </p>
                </fieldset>
                <NewsletterAddress settings />
            </div>
        </DataSettings>
    );
}
