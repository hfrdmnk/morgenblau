import { useState } from 'react';
import { HugeiconsIcon } from '@hugeicons/react';
import { Loading03Icon, Login03Icon } from '@hugeicons/core-free-icons';
import { useAuth } from '../lib/auth-context';
import { Button } from './ui/button';
import { InputGroup, InputGroupAddon, InputGroupInput, InputGroupText } from './ui/input-group';
import { Label } from './ui/label';
import { LevelContext } from '../lib/LevelContext';
import { MorgenblauLogo } from './MorgenblauLogo';
import { Window } from './Window';

const MIN_SPINNER_MS = 150;

export function LoginPage() {
	const [handle, setHandle] = useState('');
	const [error, setError] = useState('');
	const [submitting, setSubmitting] = useState(false);
	const { signIn, status } = useAuth();

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		const trimmed = handle.trim().replace(/^@/, '');
		if (!trimmed) return;

		setError('');
		setSubmitting(true);
		const startedAt = Date.now();

		try {
			await signIn(trimmed);
		} catch {
			setError('Could not sign in. Check your handle or DID and try again.');
		} finally {
			const elapsed = Date.now() - startedAt;
			if (elapsed < MIN_SPINNER_MS) {
				await new Promise((resolve) => setTimeout(resolve, MIN_SPINNER_MS - elapsed));
			}
			setSubmitting(false);
		}
	};

	const isLoading = status === 'loading' || submitting;

	return (
		<div className="grid h-dvh grid-cols-[1.618fr_1fr] bg-background p-6">
			<svg width="0" height="0" className="absolute" aria-hidden="true">
				<defs>
					<filter id="morgenblau-stamp" x="-20%" y="-20%" width="140%" height="140%">
						<feGaussianBlur in="SourceAlpha" stdDeviation="1" result="blur-top" />
						<feOffset in="blur-top" dy="1" result="offset-top" />
						<feComposite
							in="offset-top"
							in2="SourceAlpha"
							operator="arithmetic"
							k2="-1"
							k3="1"
							result="inner-top"
						/>
						<feFlood floodColor="#000" floodOpacity="0.22" result="dark-flood" />
						<feComposite in="dark-flood" in2="inner-top" operator="in" result="depression" />

						<feGaussianBlur in="SourceAlpha" stdDeviation="0.5" result="blur-bottom" />
						<feOffset in="blur-bottom" dy="-1" result="offset-bottom" />
						<feComposite
							in="offset-bottom"
							in2="SourceAlpha"
							operator="arithmetic"
							k2="-1"
							k3="1"
							result="inner-bottom"
						/>
						<feFlood floodColor="#fff" floodOpacity="0.7" result="light-flood" />
						<feComposite in="light-flood" in2="inner-bottom" operator="in" result="highlight" />

						<feMerge>
							<feMergeNode in="SourceGraphic" />
							<feMergeNode in="depression" />
							<feMergeNode in="highlight" />
						</feMerge>
					</filter>
				</defs>
			</svg>

			<div className="flex items-center justify-center">
				<LevelContext value={0}>
					<div className="w-full max-w-sm">
						<div className="flex flex-col items-center">
							<MorgenblauLogo
								className="size-14 text-gray-200 dark:text-gray-800"
								style={{ filter: 'url(#morgenblau-stamp)' }}
							/>
							<h1 className="mt-6 text-2xl tracking-tighter">Morgenblau</h1>
							<p className="mt-1 text-sm text-muted-foreground">A social RSS platform</p>
						</div>

						<form onSubmit={handleSubmit} className="mt-10 space-y-1.5">
							<Label htmlFor="handle">Your Atmosphere Account</Label>
							<div className="flex items-center gap-2">
								<InputGroup>
									<InputGroupAddon align="inline-start">
										<InputGroupText>@</InputGroupText>
									</InputGroupAddon>
									<InputGroupInput
										id="handle"
										value={handle}
										onChange={(e) => setHandle(e.target.value)}
										disabled={isLoading}
										autoCapitalize="none"
										autoCorrect="off"
										spellCheck={false}
									/>
								</InputGroup>
								<Button
									type="submit"
									size="icon"
									disabled={isLoading || !handle.trim()}
									aria-label={submitting ? 'Signing in' : 'Log in'}
								>
									<HugeiconsIcon
										icon={submitting ? Loading03Icon : Login03Icon}
										className={submitting ? 'motion-safe:animate-spin' : undefined}
									/>
								</Button>
							</div>
							{error && <p className="text-sm text-destructive">{error}</p>}
						</form>

						<div className="mt-4 font-handwritten text-sm leading-snug text-muted-foreground">
							<p>New here?</p>
							<p>
								An Atmosphere account is your identity (or user name) across Bluesky, Tangled,
								Leaflet, Grain and other AT Protocol apps.
							</p>
						</div>
					</div>
				</LevelContext>
			</div>

			<Window>
				<div className="h-full w-full bg-linear-to-b from-atmosphere-blue to-sand-brown" />
			</Window>
		</div>
	);
}
