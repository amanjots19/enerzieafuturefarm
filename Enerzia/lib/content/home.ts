/**
 * Homepage copy, lifted from the Homepage design in the Claude Design project.
 *
 * Four things differ from the design as drawn, all deliberate:
 *
 *  1. "COD available" is GONE. There is no cash on delivery — see roadmap.md
 *     §Payment ("Payment is collected by Razorpay Checkout. There is no cash on
 *     delivery") and internal/order, which records COD was removed when payment
 *     moved to Razorpay. Advertising a payment method we cannot honour produces
 *     refund disputes, not sales.
 *  2. The `[___]` free-shipping placeholders are filled from FREE_SHIPPING_* below.
 *  3. The returns FAQ no longer promises "unopened packs returned within [___]
 *     days for a full refund". Terms clause 9 says opened or used products
 *     cannot be returned and lists four specific reasons with a 48-hour claim
 *     window. The homepage must not offer a policy the Terms withhold.
 *  4. The #farm section was never built into this page — the farm got its own
 *     page instead (`/farm`, copy in ./farm.ts), which the header now links to.
 *     #benefits is still unbuilt and has no nav entry, deliberately.
 *
 * ---------------------------------------------------------------------------
 * WHICH IMAGES ON THIS PAGE MAY BE GENERATED
 *
 * The hero and the three "Living with it" cards are AI-generated still-lifes
 * (2026-08-16). That is deliberate and bounded, and the boundary matters:
 *
 *   MAY be generated — images that only illustrate *use*: a smoothie on a
 *   counter, a bottle in a bag, a shaker after training, powder in a bowl.
 *   None of them asserts anything a buyer is being asked to trust.
 *
 *   MUST be real — anything depicting the farm, the ponds, the process, the
 *   people, or the pack. This brand's entire claim is "we grew it ourselves and
 *   you can trace it". A generated pond or a generated jar would fake exactly
 *   the thing the claim rests on, and one recognised fake would take the whole
 *   claim down with it. Those live in `/assets/farm/` and are the owner's own
 *   photographs.
 *
 * Also: no generated image may show a branded pack. A generator invents a
 * label, and an invented Enerzeia jar is worse than showing no jar at all.
 * ---------------------------------------------------------------------------
 */

/**
 * Free shipping threshold, in rupees.
 *
 * SOURCE OF TRUTH IS THE SERVER: `FreeShippingThreshold int64 = 49900` (paise)
 * in enerzia-be/internal/cart/pricing.go. It reaches the client on GET /cart as
 * `freeShipping.thresholdAmount` — but that endpoint requires a signed-in
 * shopper, and the homepage is public, so the value is mirrored here.
 *
 * THIS CAN DRIFT. If the server constant changes, change it here too. The
 * durable fix is exposing the threshold on a public endpoint, which is a
 * contract change and therefore the owner's decision.
 */
import type { Photo } from './types';

export const FREE_SHIPPING_RUPEES = 499;

export const ANNOUNCEMENT = `Free shipping over ₹${FREE_SHIPPING_RUPEES}`;

export const HERO = {
  eyebrow: 'Grown on our own farm',
  /** Rendered as three lines; the breaks are intentional typesetting. */
  headingLines: ['Spirulina you can', 'trace back to', 'the water it grew in.'],
  body: "Harvested on our farm, sun-dried at low heat and lab-tested batch by batch - powder for your morning smoothie, tablets for the days you're moving.",
  ctaLabel: 'Shop spirulina - from ₹400',
  /** Floating stat over the hero image. */
  badgeBig: '62%+',
  badgeLabel: 'plant protein by weight',
  /**
   * GENERATED still-life (owner's choice, 2026-08-16), not a photograph of our
   * farm — see the note at the top of this file about which images may be
   * generated and which may not. It makes no claim about where anything was
   * grown, so it is safe here.
   *
   * `/assets/farm/pond-raceway-wide.jpg` — a real photograph of our own ponds —
   * remains in the repo and is the one-line alternative if a documentary hero
   * is ever preferred. It arguably suits the headline better, since that
   * promises spirulina "you can trace back to the water it grew in".
   *
   * The lower-left of this frame is deliberately empty: the 62%+ badge floats
   * over that corner and a busy crop there makes it unreadable.
   *
   * The gradient is retained as the load/failure fallback — see
   * `components/Photo.tsx`.
   */
  photo: {
    src: '/assets/home/hero-powder.jpg',
    alt: 'A shallow stoneware bowl heaped with deep green spirulina powder on cream linen, a brass spoon of powder beside it and a few dried spirulina strands scattered nearby.',
    gradient: 'radial-gradient(125% 100% at 25% 15%,#cfe9d5,#5f9a76 55%,#123f2d)',
  } satisfies Photo,
};

/**
 * Hero trust chips. These link into #proof, which IS built — unlike the farm
 * and benefits sections, so they are real links.
 */
/**
 * "Heavy metals tested" rather than "no heavy metals" — the owner asked for a
 * heavy-metals badge on 2026-08-16, and this is the defensible form of it. A
 * lab report shows quantities **below the permitted limit**; it cannot show
 * zero, so an absolute "no heavy metals" is a claim no test result on file
 * actually supports. On an FSSAI-licensed food that distinction is the whole
 * difference between a substantiated claim and a false one. The wording also
 * matches what is already claimed elsewhere — the PDP badge reads "Lab tested /
 * Heavy metals & microbes".
 */
export const HERO_CHIPS = [
  { label: 'FSSAI licensed', href: '#proof' },
  { label: 'Third-party lab tested', href: '#proof' },
  { label: 'Heavy metals tested', href: '#proof' },
  { label: '62%+ plant protein', href: '#proof' },
];

export interface Moment {
  when: string;
  title: string;
  body: string;
  form: string;
  photo: Photo;
}

export const LIFESTYLE_INTRO = {
  eyebrow: 'Living with it',
  heading: "A spoon in the morning, and that's the habit.",
  body: 'Spirulina is a food, so it slots into what you already eat. Three ways our buyers use it - pick the one that survives a Tuesday.',
};

/**
 * The three photographs here are GENERATED still-lifes, not documentary
 * photographs — see the note at the top of this file. They illustrate how the
 * product is used and make no claim about the farm, the process or the pack,
 * which is why generating them is acceptable where generating a pond is not.
 *
 * None of them shows a branded pack, deliberately: a generator invents a label,
 * and an invented Enerzeia jar is worse than no jar at all.
 */
export const MOMENTS: Moment[] = [
  {
    when: 'Morning',
    title: 'Into the smoothie',
    body: 'A teaspoon of powder with banana, curd or mango and it disappears - colour aside. This is how most people stick with it.',
    form: 'Powder',
    photo: {
      src: '/assets/home/morning-smoothie.jpg',
      alt: 'A tall glass of deep green smoothie on a pale kitchen counter in morning light, beside a ripe banana and a small steel bowl of spirulina powder with a spoon.',
      gradient: 'radial-gradient(120% 100% at 30% 25%,#cfe9d5,#5f9a76 55%,#123f2d)',
    },
  },
  {
    when: 'Out of the house',
    title: 'Tablets in the bag',
    body: 'Travel days, office desks, hostel rooms. No mixing, no green rim on the glass - the same spirulina, pressed.',
    form: 'Tablets',
    photo: {
      src: '/assets/home/tablets-in-bag.jpg',
      alt: 'An open canvas tote on a wooden desk with a laptop, notebook, keys and a steel water bottle, and a small amber glass bottle of green spirulina tablets resting among them.',
      gradient: 'radial-gradient(120% 100% at 65% 30%,#dfeed3,#7fa85c 55%,#2d5327)',
    },
  },
  {
    when: 'After training',
    title: 'With your protein',
    body: 'Stir the powder into a shake, or take tablets alongside it - a plant protein that carries iron and B-vitamins too.',
    form: 'Powder or tablets',
    photo: {
      src: '/assets/home/after-training.jpg',
      alt: 'A stainless steel shaker bottle on a stone ledge beside a rolled towel, with a scoop of deep green spirulina powder resting in front of it.',
      gradient: 'radial-gradient(120% 100% at 45% 70%,#e6efd9,#9ab06a 55%,#4a5526)',
    },
  },
];

export const TASTE_TIPS = [
  'Pair it with something sweet and cold - banana, mango, dates, curd.',
  'Lemon or ginger cuts the sea-green edge in juice.',
  'Add it after blending, not before; long whirring warms the drink.',
  'Cooking it is fine, but keep the heat gentle - no boiling.',
];

export const STARTING_OUT = {
  heading: 'Starting out',
  body: 'Begin with a small amount for the first week, with food, and see how you like it. Keep the pack sealed and away from light - spirulina fades in sunlight.',
  /** Mirrors the caution in Terms clause 4 — keep the two in step. */
  caution:
    'If you are pregnant, nursing, or taking medication, talk to your doctor before adding any supplement.',
};

export interface Seal {
  mark: string;
  ring: string;
  title: string;
  body: string;
}

export const PROOF = {
  heading: 'Checked before it reaches you',
  aside: 'Every batch, without exception.',
  /** Large decorative word behind the seals. */
  watermark: 'TESTED',
};

export const SEALS: Seal[] = [
  {
    mark: 'FSSAI',
    ring: 'licensed',
    title: 'FSSAI licensed',
    body: 'Own manufacturing unit, licence on every pack.',
  },
  {
    mark: 'NABL',
    ring: 'lab',
    title: 'Third-party tested',
    body: 'Independent NABL-accredited laboratory.',
  },
  {
    mark: 'micro',
    ring: 'biology',
    title: 'Microbiology',
    body: 'Total plate count, E. coli, Salmonella, yeast and mould.',
  },
  /**
   * Added with the "Heavy metals tested" hero chip, because that chip links to
   * #proof — a badge pointing at a section that does not mention the thing it
   * claims is worse than no badge. Worded as "against the permitted limits",
   * not "none present", for the reason given on HERO_CHIPS.
   */
  {
    mark: 'HM',
    ring: 'limits',
    title: 'Heavy metals',
    body: 'Lead, arsenic, cadmium and mercury, against the permitted limits.',
  },
  /**
   * Was "Microcystin screened" until 2026-08-23, when the owner asked for a
   * nutritional-value seal in its place. The wording stays inside what the
   * batch lab report actually carries — the values printed on the pack — so it
   * claims verification, not a specific number. The 62%+ protein figure is
   * claimed by the hero badge and HERO_CHIPS and is not restated here.
   */
  {
    mark: 'NUTRI',
    ring: 'value',
    title: 'Nutritional value',
    body: 'Protein and the nutrition panel on the pack, confirmed batch by batch.',
  },
];

export interface HomeFaq {
  q: string;
  a: string;
}

/**
 * The four questions the homepage answers. Deliberately a different, shorter
 * set from the eleven on /faq — this is the "before you buy" cut, and the FAQ
 * section links through to the full page.
 */
export const HOME_FAQS: HomeFaq[] = [
  {
    q: 'How much should I take in a day?',
    a: 'Most people start with 3–5 g of powder (about a teaspoon) or 4–6 tablets a day, taken with food or a drink. Start lower for the first week. If you are pregnant, nursing or on medication, check with your doctor first.',
  },
  {
    q: 'What does it taste like?',
    a: 'Spirulina is naturally mild and virtually tasteless, so it won’t alter the taste of your food or beverages. You can enjoy it in whichever form you prefer—tablets, powder, or otherwise. Its vibrant green-blue colour comes naturally from its pigments, with no added colours.',
  },
  {
    q: 'How is it shipped, and how fast?',
    a: `Dispatched from our unit in sealed, light-proof packs. Delivery times depend on your location and the courier, so the dates shown at checkout are estimates rather than guarantees. Shipping is free over ₹${FREE_SHIPPING_RUPEES}.`,
  },
  {
    q: 'What if something is wrong with my order?',
    a: 'Because these are food products, opened or used packs cannot be returned. If the wrong item arrives, or a pack is damaged, defective or past its date, tell us within 48 hours of delivery with photographs and your order number and we will replace it or refund you after checking. The full policy is in our Terms.',
  },
];
