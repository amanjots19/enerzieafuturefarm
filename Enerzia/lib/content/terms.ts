import type { LegalDoc } from './types';

/**
 * Terms & Conditions, supplied by the business owner on 2026-08-13.
 *
 * The only change from the supplied text is the contact address in clause 23:
 * the source read `supportatenerzeiafuturefarm@gamil.com` ("gamil"), and the
 * owner confirmed the monitored mailbox is `support@enerzeiafuturefarm.com`.
 * The same address is used in the Privacy Policy, the footer and the contact
 * page — see SUPPORT_EMAIL in lib/content/social.ts, the single source.
 *
 * Do not reword any clause. Legal text is the owner's to change.
 */
export const TERMS: LegalDoc = {
  title: 'Terms & Conditions',
  lastUpdated: '15 February 2026',
  intro: [
    {
      kind: 'p',
      text: 'Welcome to Enerzeia Future Farm ("Enerzeia", "we", "our", or "us"). By accessing or using our website, purchasing our products, or using our services, you agree to comply with these Terms & Conditions. If you do not agree with these Terms, please do not use our website.',
    },
  ],
  sections: [
    {
      heading: '1. About Us',
      blocks: [
        {
          kind: 'p',
          text: 'Enerzeia Future Farm is engaged in the marketing and sale of nutraceutical products, including Spirulina Tablets and related wellness products.',
        },
        {
          kind: 'p',
          text: 'Our products are manufactured in licensed facilities and comply with applicable Indian food safety regulations.',
        },
      ],
    },
    {
      heading: '2. Eligibility',
      blocks: [
        {
          kind: 'p',
          text: 'You must be at least 18 years of age or access this website under the supervision of a parent or legal guardian.',
        },
        {
          kind: 'p',
          text: 'By placing an order, you confirm that the information provided is accurate and complete.',
        },
      ],
    },
    {
      heading: '3. Product Information',
      blocks: [
        {
          kind: 'p',
          text: 'We make every effort to ensure that product descriptions, images, nutritional information, and specifications displayed on the website are accurate.',
        },
        { kind: 'p', text: 'However:' },
        {
          kind: 'ul',
          items: [
            'Product images are for illustration purposes only.',
            'Actual packaging may vary slightly.',
            'Colours may appear different depending on your screen.',
            'Specifications may change without prior notice due to product improvements.',
          ],
        },
      ],
    },
    {
      heading: '4. Nutritional Supplement Disclaimer',
      blocks: [
        { kind: 'p', text: 'Enerzeia products are nutraceutical food supplements.' },
        {
          kind: 'p',
          text: 'They are not medicines and are not intended to diagnose, treat, cure, or prevent any disease.',
        },
        {
          kind: 'p',
          text: 'Our products should be consumed as part of a balanced diet and healthy lifestyle.',
        },
        {
          kind: 'p',
          text: 'Always follow the recommended serving size mentioned on the product label.',
        },
        {
          kind: 'p',
          text: 'Individuals who are pregnant, breastfeeding, taking medication, or suffering from any medical condition should consult a qualified healthcare professional before using any supplement.',
        },
      ],
    },
    {
      heading: '5. Orders',
      blocks: [
        { kind: 'p', text: 'All orders are subject to acceptance and availability.' },
        { kind: 'p', text: 'We reserve the right to:' },
        {
          kind: 'ul',
          items: [
            'Cancel any order.',
            'Refuse service.',
            'Limit product quantities.',
            'Correct pricing errors.',
            'Reject suspicious or fraudulent orders.',
          ],
        },
      ],
    },
    {
      heading: '6. Pricing',
      blocks: [
        { kind: 'p', text: 'All prices displayed are in Indian Rupees (INR).' },
        { kind: 'p', text: 'Prices include applicable taxes unless otherwise stated.' },
        { kind: 'p', text: 'Prices may change without prior notice.' },
      ],
    },
    {
      heading: '7. Payment',
      blocks: [
        {
          kind: 'p',
          text: 'We accept secure online payments through authorized payment gateways.',
        },
        {
          kind: 'p',
          text: 'Enerzeia does not store your complete debit card, credit card, or banking information.',
        },
        {
          kind: 'p',
          text: 'Payment processing is handled by trusted third-party payment providers.',
        },
      ],
    },
    {
      heading: '8. Shipping',
      blocks: [
        {
          kind: 'p',
          text: 'Orders are processed within the estimated processing time mentioned on our website.',
        },
        { kind: 'p', text: 'Delivery timelines depend on:' },
        {
          kind: 'ul',
          items: [
            'Customer location',
            'Courier partner',
            'Weather conditions',
            'Public holidays',
            'Unexpected operational delays',
          ],
        },
        { kind: 'p', text: 'Estimated delivery dates are not guaranteed.' },
      ],
    },
    {
      heading: '9. Returns & Refunds',
      blocks: [
        {
          kind: 'p',
          text: 'Due to the nature of food and nutraceutical products, opened or used products cannot be returned.',
        },
        { kind: 'p', text: 'Returns are accepted only if:' },
        {
          kind: 'ul',
          items: [
            'Wrong product delivered',
            'Damaged product received',
            'Manufacturing defect',
            'Expired product received',
          ],
        },
        {
          kind: 'p',
          text: 'Claims must be raised within 48 hours of delivery along with clear photographs and the order number.',
        },
        {
          kind: 'p',
          text: 'Refunds, replacements, or store credits will be issued after verification.',
        },
      ],
    },
    {
      heading: '10. Cancellations',
      blocks: [
        { kind: 'p', text: 'Orders may be cancelled before dispatch.' },
        { kind: 'p', text: 'Once shipped, cancellation requests cannot be accepted.' },
      ],
    },
    {
      heading: '11. User Account',
      blocks: [
        { kind: 'p', text: 'If you create an account, you are responsible for:' },
        {
          kind: 'ul',
          items: [
            'Maintaining password confidentiality',
            'All activities under your account',
            'Providing accurate information',
          ],
        },
        {
          kind: 'p',
          text: 'We reserve the right to suspend or terminate accounts involved in misuse or fraudulent activities.',
        },
      ],
    },
    {
      heading: '12. Intellectual Property',
      blocks: [
        { kind: 'p', text: 'All content on this website, including but not limited to:' },
        {
          kind: 'ul',
          items: [
            'Logo',
            'Brand name',
            'Product designs',
            'Label artwork',
            'Images',
            'Videos',
            'Graphics',
            'Website design',
            'Product descriptions',
            'Blog content',
            'Documents',
          ],
        },
        {
          kind: 'p',
          text: 'is the exclusive property of Enerzeia Future Farm unless otherwise stated.',
        },
        {
          kind: 'p',
          text: 'No content may be copied, reproduced, distributed, modified, or used commercially without prior written permission.',
        },
      ],
    },
    {
      heading: '13. Prohibited Use',
      blocks: [
        { kind: 'p', text: 'You agree not to:' },
        {
          kind: 'ul',
          items: [
            'Use the website for unlawful purposes.',
            'Attempt unauthorized access to our systems.',
            'Upload malicious software.',
            'Copy or scrape website content.',
            'Interfere with website functionality.',
            'Violate applicable laws.',
          ],
        },
      ],
    },
    {
      heading: '14. Health Information',
      blocks: [
        {
          kind: 'p',
          text: 'Information available on this website is provided for general educational purposes only.',
        },
        { kind: 'p', text: 'It should not be considered medical advice.' },
        {
          kind: 'p',
          text: 'Always seek advice from a qualified healthcare professional regarding any medical condition or treatment.',
        },
        {
          kind: 'p',
          text: 'Never ignore professional medical advice because of information available on this website.',
        },
      ],
    },
    {
      heading: '15. Third-Party Links',
      blocks: [
        { kind: 'p', text: 'Our website may contain links to third-party websites.' },
        { kind: 'p', text: 'Enerzeia is not responsible for:' },
        {
          kind: 'ul',
          items: [
            'Third-party content',
            'Privacy practices',
            'Product quality',
            'Website accuracy',
            'External services',
          ],
        },
        { kind: 'p', text: 'Accessing external websites is at your own risk.' },
      ],
    },
    {
      heading: '16. Limitation of Liability',
      blocks: [
        {
          kind: 'p',
          text: 'To the maximum extent permitted under applicable law, Enerzeia Future Farm shall not be liable for any indirect, incidental, consequential, special, or punitive damages arising from:',
        },
        {
          kind: 'ul',
          items: [
            'Website usage',
            'Product usage',
            'Delivery delays',
            'Technical errors',
            'Service interruptions',
          ],
        },
        {
          kind: 'p',
          text: 'Our total liability shall not exceed the amount paid for the product purchased.',
        },
        {
          kind: 'p',
          text: 'Nothing in these Terms limits liability where such limitation is prohibited by law.',
        },
      ],
    },
    {
      heading: '17. Warranty',
      blocks: [
        { kind: 'p', text: 'Unless expressly stated, products are provided "as is."' },
        { kind: 'p', text: 'We do not guarantee:' },
        {
          kind: 'ul',
          items: [
            'Individual nutritional outcomes',
            'Suitability for every user',
            'Continuous website availability',
            'Error-free operation',
          ],
        },
      ],
    },
    {
      heading: '18. Reviews & User Content',
      blocks: [
        { kind: 'p', text: 'Customers may submit reviews, comments, and feedback.' },
        {
          kind: 'p',
          text: 'By submitting content, you grant Enerzeia Future Farm a non-exclusive, royalty-free, worldwide license to use, display, reproduce, and publish that content for business purposes.',
        },
        {
          kind: 'p',
          text: 'We reserve the right to remove content that is false, offensive, defamatory, unlawful, or otherwise inappropriate.',
        },
      ],
    },
    {
      heading: '19. Privacy',
      blocks: [
        { kind: 'p', text: 'Your use of our website is also governed by our Privacy Policy.' },
        {
          kind: 'p',
          text: 'By using this website, you agree to the collection and use of your information in accordance with our Privacy Policy.',
        },
      ],
    },
    {
      heading: '20. Force Majeure',
      blocks: [
        {
          kind: 'p',
          text: 'Enerzeia shall not be liable for delays or failure to perform obligations caused by events beyond our reasonable control, including natural disasters, floods, fires, epidemics, strikes, government actions, internet outages, or transportation disruptions.',
        },
      ],
    },
    {
      heading: '21. Governing Law & Jurisdiction',
      blocks: [
        {
          kind: 'p',
          text: 'These Terms & Conditions shall be governed by the laws of India.',
        },
        {
          kind: 'p',
          text: 'Any disputes arising out of or relating to these Terms shall be subject to the exclusive jurisdiction of the competent courts located in Rohtak, Haryana, unless otherwise required by applicable law.',
        },
      ],
    },
    {
      heading: '22. Changes to These Terms',
      blocks: [
        {
          kind: 'p',
          text: 'We reserve the right to update or modify these Terms & Conditions at any time.',
        },
        {
          kind: 'p',
          text: 'Updated versions will be published on this page with the revised "Last Updated" date.',
        },
        {
          kind: 'p',
          text: 'Continued use of the website after changes are posted constitutes acceptance of the revised Terms.',
        },
      ],
    },
    {
      heading: '23. Contact Information',
      blocks: [
        {
          kind: 'p',
          text: 'If you have any questions regarding these Terms & Conditions, please contact us:',
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
  ],
};
