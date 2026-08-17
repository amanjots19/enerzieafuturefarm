import type { Metadata } from 'next';

import { LegalDocPage } from '@/components/LegalDocPage';
import { PRIVACY } from '@/lib/content/privacy';

export const metadata: Metadata = {
  title: 'Privacy Policy | Enerzeia Future Farm',
  description: 'Privacy Policy for Enerzeia Future Farm — how we collect, use and protect your information.',
};

export default function PrivacyPage() {
  return <LegalDocPage doc={PRIVACY} />;
}
