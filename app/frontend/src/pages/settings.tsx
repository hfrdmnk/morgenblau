import { NewsletterAddress } from '@/components/newsletter-address';
import { Link } from '@/components/ui/link';
import { RadioCard } from '@/components/ui/radio-card';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { readerFontLabels, readerFonts, useReaderFont } from '@/hooks/use-reader-font';
import { DataSettings } from '@/layouts/data-settings';

const SPECIMEN_FAMILY = { sans: 'sans', serif: 'serif', mono: 'duo' } as const;

export function Settings() {
    const [readerFont, setReaderFont, fontError] = useReaderFont();
    useDocumentTitle('General settings');

    return (
        <DataSettings>
            <Frame gap="5xl">
                <Text as="h1" size="2xl" weight="medium">
                    General settings
                </Text>
                <Frame maxWidth="xl" gap="5xl">
                    {/* The legend sits outside a fieldset's flex box, so the gap starts below the hint. */}
                    <Frame as="fieldset" gap="xl">
                        <legend>
                            <Text as="span" display="block" size="lg" weight="medium">
                                Reader font
                            </Text>
                        </legend>
                        <Frame paddingTop="2xs">
                            {fontError ? (
                                <Text role="alert" size="sm" tone="destructive">
                                    {fontError}
                                </Text>
                            ) : (
                                <Text size="sm" tone="muted">
                                    Saved on this device.
                                </Text>
                            )}
                        </Frame>
                        <Frame gap="m">
                            <Frame display="grid" columns={3} gap="m">
                                {readerFonts.map((font) => (
                                    <RadioCard
                                        key={font}
                                        name="reader-font"
                                        value={font}
                                        checked={readerFont === font}
                                        onChange={() => setReaderFont(font)}
                                    >
                                        <Text
                                            as="span"
                                            aria-hidden="true"
                                            size="3xl"
                                            truncate
                                            family={SPECIMEN_FAMILY[font]}
                                        >
                                            Aa Bb Cc Dd Ee Ff Gg Hh Ii Jj Kk Ll Mm Nn Oo Pp Qq Rr Ss Tt Uu Vv Ww Xx Yy
                                            Zz
                                        </Text>
                                        <Text as="span" size="sm" weight="medium">
                                            {readerFontLabels[font]}
                                        </Text>
                                    </RadioCard>
                                ))}
                            </Frame>
                            <Text size="xs" tone="subtle">
                                Geist is designed by{' '}
                                <Link href="https://vercel.com" rel="noopener noreferrer" target="_blank">
                                    Vercel
                                    <Text as="span" visuallyHidden>
                                        {' '}
                                        (opens in a new tab)
                                    </Text>
                                </Link>
                                , Newsreader by{' '}
                                <Link href="https://productiontype.com" rel="noopener noreferrer" target="_blank">
                                    Production Type
                                    <Text as="span" visuallyHidden>
                                        {' '}
                                        (opens in a new tab)
                                    </Text>
                                </Link>{' '}
                                and iA Writer Duo by{' '}
                                <Link href="https://ia.net" rel="noopener noreferrer" target="_blank">
                                    Information Architects
                                    <Text as="span" visuallyHidden>
                                        {' '}
                                        (opens in a new tab)
                                    </Text>
                                </Link>
                                .
                            </Text>
                        </Frame>
                    </Frame>
                    <NewsletterAddress settings />
                </Frame>
            </Frame>
        </DataSettings>
    );
}
