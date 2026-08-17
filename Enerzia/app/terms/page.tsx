import type { Metadata } from 'next';

import { LegalDocPage } from '@/components/LegalDocPage';
import { TERMS } from '@/lib/content/terms';

export const metadata: Metadata = {
  title: 'Terms & Conditions | Enerzeia Future Farm',
  description: 'Terms and Conditions for purchasing from Enerzeia Future Farm.',
};

export default function TermsPage() {
  return <LegalDocPage doc={TERMS} />;
}
