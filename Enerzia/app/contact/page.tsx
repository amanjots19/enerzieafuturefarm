import type { Metadata } from 'next';
import Link from 'next/link';

import { ContactChannels } from '@/components/ContactChannels';
import { POSTAL_ADDRESS } from '@/lib/content/social';
import { pageMetadata } from '@/lib/seo';

export const metadata: Metadata = pageMetadata({
  path: '/contact',
  title: 'Contact us | Enerzeia Future Farm',
  description: 'Reach Enerzeia Future Farm on WhatsApp or by email. Questions about an order, a product, or delivery - we are here to help.',
});

export default function ContactPage() {
  return (
    <div className="wrap-sm screen">
      <h1 className="page-title">Contact us</h1>
      <p className="contact-intro">
        Questions about an order, a product, or delivery - reach us however suits you.
      </p>

      <ContactChannels />

      <p className="contact-address">{POSTAL_ADDRESS}</p>

      <p className="contact-back">
        <Link href="/shop">Back to shop</Link>
      </p>
    </div>
  );
}
