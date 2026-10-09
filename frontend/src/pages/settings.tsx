import { NewsletterAddress } from '@/components/newsletter-address';
import { useReaderFont } from '@/hooks/use-reader-font';
import { DataSettings } from '@/layouts/data-settings';
import { cn } from '@/lib/utils';

export function Settings() {
    const [readerFont, setReaderFont, fontError] = useReaderFont();

    return (
        <DataSettings>
            <h1 className="text-2xl font-medium">General settings</h1>
            <div className="mt-12 max-w-xl space-y-12">
                <fieldset>
                    <legend className="text-lg font-medium">Reader font</legend>
                    <p className="mt-1 text-sm text-muted-foreground">
                        Choose how article text feels. Titles and details stay the same.
                    </p>
                    <div className="mt-5 grid grid-cols-2 gap-3">
                        {(['sans', 'serif'] as const).map((font) => (
                            <label key={font} className="relative cursor-pointer">
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
                                    <span aria-hidden="true" className={cn('truncate text-3xl', font === 'serif' ? 'font-serif' : 'font-sans')}>
                                        Aa Bb Cc Dd Ee Ff Gg Hh Ii Jj Kk Ll Mm Nn Oo Pp Qq Rr Ss Tt Uu Vv Ww Xx Yy Zz
                                    </span>
                                    <span className="text-sm font-medium">{font === 'sans' ? 'Sans' : 'Serif'}</span>
                                </span>
                            </label>
                        ))}
                    </div>
                    {fontError ? (
                        <p role="alert" className="mt-3 text-xs text-destructive">{fontError}</p>
                    ) : (
                        <p className="mt-3 text-xs text-muted-foreground">Saved on this device.</p>
                    )}
                </fieldset>
                <NewsletterAddress settings />
            </div>
        </DataSettings>
    );
}
