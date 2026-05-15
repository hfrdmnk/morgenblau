import { useDocumentTitle } from '@/hooks/use-document-title'

// Per SPEC <daily-digests>: empty-edition placeholder until the digest renderer lands.
export function Consume() {
  useDocumentTitle('Consume')
  return (
    <main className="min-h-screen flex items-center justify-center p-8">
      <p className="text-lg text-gray-700">
        Nothing new this morning. Enjoy your coffee.
      </p>
    </main>
  )
}
