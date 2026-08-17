import type { FaqItem } from './types';

/**
 * Frequently asked questions, supplied by the business owner on 2026-08-13.
 *
 * One change from the supplied text: the final question arrived pasted from a
 * WhatsApp thread and carried a chat header —
 * "[03/08/26, 8:52:06 PM] Vishal: Can I take Spirulina every day?".
 * The timestamp and sender were stripped and it is rendered as question 11.
 *
 * Health wording here is deliberately cautious: nothing claims a therapeutic
 * effect, and the "consult a healthcare professional" caveat in Q3 mirrors
 * clause 4 of the Terms. Keep it that way — these are food supplements, not
 * medicines, and a stronger claim would contradict our own Terms.
 */
export const FAQS: FaqItem[] = [
  {
    question: 'What is Spirulina?',
    answer: [
      {
        kind: 'p',
        text: 'Spirulina is a natural blue-green algae that is rich in plant-based protein, vitamins, minerals, antioxidants, and chlorophyll. It has been consumed worldwide as a nutritious superfood and is an easy way to support your daily nutritional needs.',
      },
    ],
  },
  {
    question: 'Why should I choose Enerzeia Spirulina Tablets?',
    answer: [
      {
        kind: 'p',
        text: 'Enerzeia Spirulina Tablets are made from carefully cultivated premium-quality spirulina. Every batch is produced under strict quality standards and tested for safety to ensure purity and consistency. Our focus is to provide clean, natural nutrition that you can trust every day.',
      },
    ],
  },
  {
    question: 'Who can take Spirulina Tablets?',
    answer: [
      {
        kind: 'p',
        text: 'Enerzeia Spirulina Tablets are suitable for most healthy adults. They are commonly used by working professionals, students, fitness enthusiasts, vegetarians, and anyone looking to add more natural nutrients to their daily routine.',
      },
      {
        kind: 'p',
        text: 'If you are pregnant, breastfeeding, taking medication, or have a medical condition, please consult your healthcare professional before use.',
      },
    ],
  },
  {
    question: 'How should I take Enerzeia Spirulina Tablets?',
    answer: [
      {
        kind: 'p',
        text: 'Take 2 tablets daily with water after a meal or follow the advice of your healthcare professional. Consistent daily use is recommended as part of a balanced diet and healthy lifestyle.',
      },
    ],
  },
  {
    question: 'How long does it take to notice the benefits?',
    answer: [
      {
        kind: 'p',
        text: "Everyone's body responds differently. Most people choose spirulina as a long-term nutritional supplement rather than for immediate results. Regular daily use, along with a healthy lifestyle, provides the best overall experience.",
      },
    ],
  },
  {
    question: 'Are Enerzeia Spirulina Tablets safe?',
    answer: [
      {
        kind: 'p',
        text: 'Yes. Our spirulina is produced under hygienic manufacturing practices and every batch undergoes quality testing, including checks for heavy metals, microbiological safety, and pesticide residues, before reaching our customers.',
      },
    ],
  },
  {
    question: 'Is Spirulina suitable for vegetarians and vegans?',
    answer: [
      {
        kind: 'p',
        text: 'Yes. Enerzeia Spirulina Tablets are made from 100% plant-based spirulina, making them suitable for both vegetarians and vegans.',
      },
    ],
  },
  {
    question: 'Does Spirulina contain artificial colours, flavours, or preservatives?',
    answer: [
      {
        kind: 'p',
        text: 'No. Enerzeia Spirulina Tablets are made using natural spirulina without added artificial colours or artificial flavours. Please refer to the product label for complete ingredient information.',
      },
    ],
  },
  {
    question: 'How should I store the tablets?',
    answer: [
      {
        kind: 'p',
        text: 'Store the bottle in a cool, dry place away from direct sunlight and moisture. Always keep the lid tightly closed after use and keep the product out of reach of children.',
      },
    ],
  },
  {
    question: 'How can I verify that my product is genuine?',
    answer: [
      {
        kind: 'p',
        text: 'Every Enerzeia product includes manufacturing details, batch information, and a QR code on the label. You can scan the QR code or visit our official website to verify product information and contact our customer support team if you have any questions.',
      },
    ],
  },
  {
    question: 'Can I take Spirulina every day?',
    answer: [
      {
        kind: 'p',
        text: 'Yes. Spirulina is commonly consumed as a daily nutritional supplement. Following the recommended serving size consistently is the best way to include it in your daily routine.',
      },
    ],
  },
];
