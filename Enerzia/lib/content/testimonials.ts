import type { Testimonial } from './types';

/**
 * Customer testimonials, supplied by the business owner on 2026-08-13 and
 * confirmed by them as real customer feedback published with names withheld.
 *
 * That confirmation is why the "Verified Customer" attribution and the star
 * ratings are shown. Do NOT add entries that are not genuine customer
 * feedback, and do not let marketing copy in here wearing a star rating:
 * presenting invented reviews as customer testimony is a consumer-protection
 * problem in India (CCPA misleading-advertisement rules, BIS IS 19000:2022 on
 * online consumer reviews), not merely a matter of taste. If a claim is ever
 * challenged, the substantiation is the original feedback the owner holds.
 *
 * Unrelated but adjacent: the PDP rating ("4.8 · 312 reviews") is hardcoded
 * static content, not a real aggregate — see product.md §5, which records that
 * there is no reviews feature. That number is on the same footing as this list
 * and should be revisited before launch.
 */
export const TESTIMONIALS: Testimonial[] = [
  {
    rating: 5,
    headline: 'A simple addition to my daily routine.',
    body: 'I was looking for a clean, plant-based supplement that was easy to take every day. Enerzeia Spirulina Tablets fit perfectly into my routine. The quality of the packaging and the product gives me confidence.',
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Premium quality from packaging to product.',
    body: 'The bottle feels premium, and the tablets are neatly packed. I appreciate that the product focuses on quality testing and transparency. It gives me peace of mind while choosing a daily supplement.',
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Exactly what I was looking for.',
    body: "I wanted a natural supplement without unnecessary ingredients. Enerzeia Spirulina Tablets have become part of my daily wellness routine, and I'm happy with the overall quality.",
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Trustworthy and professionally made.',
    body: 'What impressed me most was the attention to detail-from the label to the safety information. It feels like a brand that genuinely cares about quality.',
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Easy to include in my lifestyle.',
    body: "The tablets are convenient to carry and simple to take every day. It's a practical option for anyone looking to support a balanced lifestyle.",
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Clean, premium, and reliable.',
    body: 'I always check product quality before buying supplements. Enerzeia stands out with its professional presentation and emphasis on testing and safety.',
    attribution: 'Verified Customer',
  },
  {
    rating: 5,
    headline: 'Highly satisfied with my purchase.',
    body: 'The ordering experience, packaging, and overall product quality met my expectations. I would definitely consider buying from Enerzeia Future Farm again.',
    attribution: 'Verified Customer',
  },
];
