import type { Metadata } from 'next';

import { LegalDocPage } from '@/components/LegalDocPage';
import { TERMS } from '@/lib/content/terms';
import { pageMetadata } from '@/lib/seo';

export const metadata: Metadata = pageMetadata({
  path: '/terms',
  title: 'Terms & Conditions | Enerzeia Future Farm',
  description: 'Terms and Conditions for purchasing from Enerzeia Future Farm.',
});

export default function TermsPage() {
  return <LegalDocPage doc={TERMS} />;
}
