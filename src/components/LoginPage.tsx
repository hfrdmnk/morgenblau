import { useState } from 'react';
import { HugeiconsIcon } from '@hugeicons/react';
import { Loading03Icon, Login03Icon } from '@hugeicons/core-free-icons';
import { useAuth } from '../lib/auth-context';
import { Button } from './ui/button';
import { InputGroup, InputGroupAddon, InputGroupInput, InputGroupText } from './ui/input-group';
import { Label } from './ui/label';
import { Card } from './Card';
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
			<div className="flex items-center">
				<Card level={1} className="w-full max-w-xl p-8">
					<h1 className="text-2xl font-semibold tracking-tighter">Morgenblau</h1>
					<p className="mt-1 text-sm text-muted-foreground">A social RSS platform</p>

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
				</Card>
			</div>

			<Window>
				<div className="h-full w-full bg-linear-to-b from-atmosphere-blue to-sand-brown" />
			</Window>
		</div>
	);
}
