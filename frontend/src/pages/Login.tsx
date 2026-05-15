// Native form submit — no fetch, no preventDefault. The browser must navigate
// to follow the AS redirect that /oauth/login responds with.
export function Login() {
  return (
    <main className="min-h-screen flex items-center justify-center p-8">
      <form
        method="POST"
        action="/oauth/login"
        className="w-full max-w-sm space-y-4"
      >
        <label className="block">
          <span className="block text-sm font-medium text-gray-700">Handle</span>
          <input
            name="handle"
            type="text"
            required
            autoComplete="off"
            spellCheck={false}
            placeholder="alice.bsky.social"
            className="mt-1 block w-full rounded border border-gray-300 px-3 py-2"
          />
        </label>
        <button
          type="submit"
          className="w-full rounded bg-gray-900 px-4 py-2 text-white"
        >
          Sign in
        </button>
      </form>
    </main>
  )
}
