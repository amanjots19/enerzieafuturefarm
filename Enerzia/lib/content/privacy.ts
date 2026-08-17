import type { LegalDoc } from './types';

/**
 * Privacy Policy, supplied by the business owner on 2026-08-13.
 *
 * Two changes from the supplied text, both agreed with the owner:
 *   - "Last Updated" was the template placeholder "[DD Month YYYY]". Set to
 *     the date this policy was first published. UPDATE THIS whenever the text
 *     below changes — clause 13 promises the date tracks revisions.
 *   - The contact address in clause 14 read
 *     `supportatenerzeiafuturefarm@gmail.com.com` (doubled ".com"); the owner
 *     confirmed the monitored mailbox is `support@enerzeiafuturefarm.com`.
 *
 * Do not reword any clause. Legal text is the owner's to change.
 */
export const PRIVACY: LegalDoc = {
  title: 'Privacy Policy',
  lastUpdated: '13 August 2026',
  intro: [
    {
      kind: 'p',
      text: 'Welcome to Enerzeia Future Farm ("Enerzeia", "we", "our", or "us"). Your privacy is important to us. This Privacy Policy explains how we collect, use, store, share, and protect your personal information when you visit our website, purchase our products, or interact with our services.',
    },
    {
      kind: 'p',
      text: 'By using our website, you agree to the practices described in this Privacy Policy.',
    },
  ],
  sections: [
    {
      heading: '1. Information We Collect',
      blocks: [
        {
          kind: 'p',
          text: 'Depending on how you use our website, we may collect the following information:',
        },
        { kind: 'p', text: 'Personal Information' },
        {
          kind: 'ul',
          items: [
            'Full Name',
            'Mobile Number',
            'Email Address',
            'Billing Address',
            'Shipping Address',
            'PIN Code',
            'Company Name (if applicable)',
          ],
        },
        { kind: 'p', text: 'Order Information' },
        {
          kind: 'ul',
          items: [
            'Products purchased',
            'Order history',
            'Payment status',
            'Invoice details',
            'Delivery information',
          ],
        },
        { kind: 'p', text: 'Technical Information' },
        {
          kind: 'ul',
          items: [
            'IP Address',
            'Browser type',
            'Device information',
            'Operating system',
            'Time zone',
            'Website usage information',
            'Cookies and similar technologies',
          ],
        },
        { kind: 'p', text: 'Customer Support Information' },
        { kind: 'p', text: 'If you contact us, we may collect:' },
        {
          kind: 'ul',
          items: [
            'Emails',
            'Phone conversations',
            'WhatsApp messages',
            'Contact form submissions',
            'Customer feedback',
            'Product reviews',
          ],
        },
      ],
    },
    {
      heading: '2. How We Use Your Information',
      blocks: [
        { kind: 'p', text: 'We use your information to:' },
        {
          kind: 'ul',
          items: [
            'Process and deliver your orders',
            'Verify transactions',
            'Send order confirmations and shipping updates',
            'Respond to customer inquiries',
            'Improve our products and website',
            'Prevent fraud and unauthorized activities',
            'Maintain business records',
            'Comply with legal obligations',
            'Send promotional communications (only where permitted or with your consent)',
          ],
        },
      ],
    },
    {
      heading: '3. Payment Information',
      blocks: [
        { kind: 'p', text: 'For your security:' },
        {
          kind: 'ul',
          items: [
            'We do not store your complete debit card, credit card, UPI PIN, CVV, or internet banking credentials.',
            'Payments are processed through secure third-party payment gateway providers.',
            'Payment information is handled according to the security standards of those providers.',
          ],
        },
      ],
    },
    {
      heading: '4. Cookies',
      blocks: [
        { kind: 'p', text: 'Our website may use cookies to:' },
        {
          kind: 'ul',
          items: [
            'Remember your preferences',
            'Improve website performance',
            'Analyze visitor traffic',
            'Maintain login sessions',
            'Enhance your shopping experience',
          ],
        },
        {
          kind: 'p',
          text: 'You can disable cookies through your browser settings. Some website features may not function properly if cookies are disabled.',
        },
      ],
    },
    {
      heading: '5. Marketing Communications',
      blocks: [
        {
          kind: 'p',
          text: 'With your consent or as otherwise permitted by law, we may send:',
        },
        {
          kind: 'ul',
          items: [
            'Product updates',
            'Promotional offers',
            'New launches',
            'Newsletters',
            'Educational content',
            'Festival offers',
          ],
        },
        {
          kind: 'p',
          text: 'You can unsubscribe from marketing emails at any time by clicking the unsubscribe link or contacting us.',
        },
      ],
    },
    {
      heading: '6. Sharing of Information',
      blocks: [
        { kind: 'p', text: 'We do not sell or rent your personal information.' },
        {
          kind: 'p',
          text: 'We may share information only with trusted service providers when necessary, such as:',
        },
        {
          kind: 'ul',
          items: [
            'Courier and logistics partners',
            'Payment gateway providers',
            'IT and website service providers',
            'Customer support platforms',
            'Marketing service providers',
            'Government or regulatory authorities when required by law',
          ],
        },
        {
          kind: 'p',
          text: 'These service providers are expected to use your information only for the services they perform for us.',
        },
      ],
    },
    {
      heading: '7. Data Security',
      blocks: [
        {
          kind: 'p',
          text: 'We take reasonable technical and organizational measures to protect your personal information from unauthorized access, disclosure, alteration, or destruction.',
        },
        {
          kind: 'p',
          text: 'However, no method of internet transmission or electronic storage is completely secure, and we cannot guarantee absolute security.',
        },
      ],
    },
    {
      heading: '8. Data Retention',
      blocks: [
        {
          kind: 'p',
          text: 'We retain your personal information only for as long as necessary to:',
        },
        {
          kind: 'ul',
          items: [
            'Complete your orders',
            'Provide customer support',
            'Maintain legal and financial records',
            'Resolve disputes',
            'Meet regulatory requirements',
          ],
        },
        {
          kind: 'p',
          text: 'When no longer required, your information will be securely deleted or anonymized, unless we are required by law to retain it.',
        },
      ],
    },
    {
      heading: '9. Your Rights',
      blocks: [
        { kind: 'p', text: 'Subject to applicable law, you may have the right to:' },
        {
          kind: 'ul',
          items: [
            'Access your personal information',
            'Correct inaccurate information',
            'Request deletion of your information',
            'Update your contact details',
            'Withdraw consent where processing is based on consent',
            'Opt out of marketing communications',
          ],
        },
        {
          kind: 'p',
          text: 'To exercise these rights, please contact us using the details provided below.',
        },
      ],
    },
    {
      heading: "10. Children's Privacy",
      blocks: [
        {
          kind: 'p',
          text: 'Our website and products are not intended for children under the age of 18.',
        },
        {
          kind: 'p',
          text: 'We do not knowingly collect personal information from children. If we become aware that such information has been collected, we will take appropriate steps to delete it.',
        },
      ],
    },
    {
      heading: '11. Third-Party Websites',
      blocks: [
        {
          kind: 'p',
          text: 'Our website may contain links to third-party websites, such as payment gateways or social media platforms.',
        },
        {
          kind: 'p',
          text: 'We are not responsible for the privacy practices, content, or security of those websites. We encourage you to review their privacy policies before sharing any personal information.',
        },
      ],
    },
    {
      heading: '12. International Data Transfers',
      blocks: [
        {
          kind: 'p',
          text: 'If your information is processed or stored outside India by our service providers, we will take reasonable steps to ensure that it receives an appropriate level of protection in accordance with applicable laws.',
        },
      ],
    },
    {
      heading: '13. Changes to This Privacy Policy',
      blocks: [
        {
          kind: 'p',
          text: 'We may update this Privacy Policy from time to time to reflect changes in our practices or legal requirements.',
        },
        {
          kind: 'p',
          text: 'The revised version will be posted on this page with the updated "Last Updated" date. Continued use of the website after changes are posted constitutes acceptance of the revised Privacy Policy.',
        },
      ],
    },
    {
      heading: '14. Contact Us',
      blocks: [
        {
          kind: 'p',
          text: 'If you have any questions, concerns, or requests regarding this Privacy Policy or the way your personal information is handled, please contact us:',
        },
        { kind: 'p', text: 'Enerzeia Future Farm' },
        {
          kind: 'ul',
          items: [
            'Email: support@enerzeiafuturefarm.com',
            'Address: Village Saimpal, Kalanaur, Rohtak, Haryana – 124113, India',
          ],
        },
      ],
    },
    {
      heading: '15. Consent',
      blocks: [
        {
          kind: 'p',
          text: 'By using our website, placing an order, or providing your personal information, you acknowledge that you have read and understood this Privacy Policy and consent to the collection, use, and disclosure of your information as described herein.',
        },
      ],
    },
  ],
};
